package chat

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"sync"
	"testing"
	"time"

	"xiaozhi-esp32-server-golang/internal/app/server/mqtt_udp"
	types_conn "xiaozhi-esp32-server-golang/internal/app/server/types"
	data_client "xiaozhi-esp32-server-golang/internal/data/client"
	msgdata "xiaozhi-esp32-server-golang/internal/data/msg"
	"xiaozhi-esp32-server-golang/internal/util"
)

func TestShouldSendSpeakRequestSkipsActiveConversationSignals(t *testing.T) {
	now := time.Now()

	t.Run("listen phase", func(t *testing.T) {
		manager, _ := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)
		manager.clientState.SetListenPhase(data_client.ListenPhaseListening)
		if manager.shouldSendSpeakRequest(now) {
			t.Fatal("expected active listen phase to skip speak_request")
		}
	})

	t.Run("client status", func(t *testing.T) {
		manager, _ := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)
		manager.clientState.SetStatus(data_client.ClientStatusLLMStart)
		if manager.shouldSendSpeakRequest(now) {
			t.Fatal("expected llmStart status to skip speak_request")
		}
	})

	t.Run("tts active", func(t *testing.T) {
		manager, _ := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)
		manager.session = &ChatSession{ttsManager: &TTSManager{}}
		manager.session.ttsManager.ttsActive.Store(true)
		if manager.shouldSendSpeakRequest(now) {
			t.Fatal("expected active TTS turn to skip speak_request")
		}
	})
}

func TestShouldSendSpeakRequestSkipsWarmPathWithinReuseWindow(t *testing.T) {
	manager, _ := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)
	manager.lastSpeakPathWarmAt.Store(time.Now().Add(-defaultSpeakRequestReuseWindow + time.Second).UnixMilli())

	if manager.shouldSendSpeakRequest(time.Now()) {
		t.Fatal("expected warm speak path within reuse window to skip speak_request")
	}
}

func TestShouldSendSpeakRequestUsesUDPBindingLastActive(t *testing.T) {
	manager, conn := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)
	conn.udpSession = &mqtt_udp.UdpSession{
		LastActive: time.Now().Add(-defaultSpeakRequestReuseWindow + time.Second),
	}

	if !manager.shouldSendSpeakRequest(time.Now()) {
		t.Fatal("expected missing UDP binding to require speak_request")
	}

	conn.udpSession.SetRemoteAddr(&net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9527})
	if manager.shouldSendSpeakRequest(time.Now()) {
		t.Fatal("expected active UDP binding within reuse window to skip speak_request")
	}
}

func TestPrepareSpeakPathForInjectedSpeechSendsSpeakRequestAndWaitsForReady(t *testing.T) {
	manager, conn := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)

	errCh := make(chan error, 1)
	go func() {
		errCh <- manager.prepareSpeakPathForInjectedSpeech("主动播报内容")
	}()

	serverMsg := waitForServerMessage(t, conn, 0)
	if serverMsg.Type != msgdata.ServerMessageTypeSpeakRequest {
		t.Fatalf("expected speak_request, got %s", serverMsg.Type)
	}
	if serverMsg.Text != "主动播报内容" {
		t.Fatalf("expected speak_request text to be forwarded, got %q", serverMsg.Text)
	}
	if serverMsg.AutoListen == nil || *serverMsg.AutoListen {
		t.Fatal("expected speak_request auto_listen=false")
	}

	if err := manager.HandleSpeakReadyMessage(&data_client.ClientMessage{
		Type:      msgdata.MessageTypeSpeakReady,
		SessionID: manager.clientState.SessionID,
		State:     msgdata.MessageStateReady,
		SpeakUDPConfig: &data_client.SpeakReadyUDPConfig{
			Ready: true,
		},
	}); err != nil {
		t.Fatalf("HandleSpeakReadyMessage returned error: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("prepareSpeakPathForInjectedSpeech returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("prepareSpeakPathForInjectedSpeech did not unblock after speak_ready")
	}

	if manager.lastSpeakPathWarmAt.Load() == 0 {
		t.Fatal("expected speak_ready to mark speak path warm")
	}
	if manager.pendingSpeakRequest != nil {
		t.Fatal("expected pending speak_request to be cleared after speak_ready")
	}
}

func TestPrepareSpeakPathForInjectedSpeechReusesPendingSpeakRequest(t *testing.T) {
	manager, conn := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)

	errCh1 := make(chan error, 1)
	errCh2 := make(chan error, 1)
	go func() {
		errCh1 <- manager.prepareSpeakPathForInjectedSpeech("第一次播报")
	}()

	_ = waitForServerMessage(t, conn, 0)

	go func() {
		errCh2 <- manager.prepareSpeakPathForInjectedSpeech("第二次播报")
	}()

	time.Sleep(30 * time.Millisecond)
	if count := conn.sentCmdCount(); count != 1 {
		t.Fatalf("expected only one pending speak_request, got %d commands", count)
	}

	if err := manager.HandleSpeakReadyMessage(&data_client.ClientMessage{
		Type:      msgdata.MessageTypeSpeakReady,
		SessionID: manager.clientState.SessionID,
		State:     msgdata.MessageStateReady,
		SpeakUDPConfig: &data_client.SpeakReadyUDPConfig{
			Ready: true,
		},
	}); err != nil {
		t.Fatalf("HandleSpeakReadyMessage returned error: %v", err)
	}

	for idx, errCh := range []chan error{errCh1, errCh2} {
		select {
		case err := <-errCh:
			if err != nil {
				t.Fatalf("waiter %d returned error: %v", idx+1, err)
			}
		case <-time.After(time.Second):
			t.Fatalf("waiter %d did not unblock after shared speak_ready", idx+1)
		}
	}
}

func TestPrepareSpeakPathForInjectedSpeechTimesOut(t *testing.T) {
	manager, conn := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)
	manager.speakReadyTimeout = 30 * time.Millisecond

	err := manager.prepareSpeakPathForInjectedSpeech("超时播报")
	if err == nil {
		t.Fatal("expected prepareSpeakPathForInjectedSpeech to time out")
	}
	if !errors.Is(err, context.DeadlineExceeded) && err.Error() != "等待 speak_ready 超时" {
		t.Fatalf("expected speak_ready timeout error, got %v", err)
	}
	if conn.sentCmdCount() != 1 {
		t.Fatalf("expected one speak_request before timeout, got %d", conn.sentCmdCount())
	}
	if manager.pendingSpeakRequest != nil {
		t.Fatal("expected timeout to clear pending speak_request")
	}
}

func TestInjectMessageWithSkipLlmFalseStillSendsSpeakRequest(t *testing.T) {
	manager, conn := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)
	manager.session = &ChatSession{
		clientState:   manager.clientState,
		chatTextQueue: util.NewQueue[AsrResponseChannelItem](1),
	}

	errCh := make(chan error, 1)
	go func() {
		errCh <- manager.InjectMessage("需要先过LLM", false)
	}()

	serverMsg := waitForServerMessage(t, conn, 0)
	if serverMsg.Type != msgdata.ServerMessageTypeSpeakRequest {
		t.Fatalf("expected speak_request, got %s", serverMsg.Type)
	}

	if err := manager.HandleSpeakReadyMessage(&data_client.ClientMessage{
		Type:      msgdata.MessageTypeSpeakReady,
		SessionID: manager.clientState.SessionID,
		State:     msgdata.MessageStateReady,
		SpeakUDPConfig: &data_client.SpeakReadyUDPConfig{
			Ready: true,
		},
	}); err != nil {
		t.Fatalf("HandleSpeakReadyMessage returned error: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil {
			t.Fatalf("InjectMessage returned error: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("InjectMessage did not return after speak_ready")
	}

	item, err := manager.session.chatTextQueue.Pop(context.Background(), time.Duration(-1))
	if err != nil {
		t.Fatalf("expected injected message to enter chat queue, got %v", err)
	}
	if item.text != "需要先过LLM" {
		t.Fatalf("expected injected text to be queued, got %q", item.text)
	}
}

func TestAddAsrResultToQueueWithOptionsCarriesPlaybackStartHook(t *testing.T) {
	manager, _ := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)
	session := &ChatSession{
		clientState:   manager.clientState,
		chatTextQueue: util.NewQueue[AsrResponseChannelItem](1),
	}

	startedCount := 0
	if err := session.AddAsrResultToQueueWithOptions("需要走LLM", nil, llmResponseChannelOptions{
		onTTSPlaybackStart: func() {
			startedCount++
		},
	}); err != nil {
		t.Fatalf("AddAsrResultToQueueWithOptions returned error: %v", err)
	}

	item, err := session.chatTextQueue.Pop(context.Background(), time.Duration(-1))
	if err != nil {
		t.Fatalf("expected queued asr item, got %v", err)
	}

	hook := ttsPlaybackStartHookFromContext(item.ctx)
	if hook == nil {
		t.Fatal("expected playback start hook to be carried in queued ctx")
	}
	if startedCount != 0 {
		t.Fatal("expected playback start hook to stay idle before TTS really starts")
	}

	hook()
	hook()
	if startedCount != 1 {
		t.Fatalf("expected playback start hook to fire once, got %d", startedCount)
	}
}

func TestAddTextToTTSQueueWithOptionsKeepsPlaybackHookOutOfQueueStart(t *testing.T) {
	manager, conn := newSpeakRequestTestManager(types_conn.TransportTypeMqttUdp)
	ttsManager := NewTTSManager(manager.clientState, NewServerTransport(conn, manager.clientState), nil)
	llmManager := NewLLMManager(manager.clientState, NewServerTransport(conn, manager.clientState), ttsManager, nil, nil)

	startedCount := 0
	if err := llmManager.AddTextToTTSQueueWithOptions("直接播报", llmResponseChannelOptions{
		onTTSPlaybackStart: func() {
			startedCount++
		},
	}); err != nil {
		t.Fatalf("AddTextToTTSQueueWithOptions returned error: %v", err)
	}

	item, err := llmManager.llmResponseQueue.Pop(context.Background(), time.Duration(-1))
	if err != nil {
		t.Fatalf("expected queued llm item, got %v", err)
	}

	if item.onStartFunc == nil {
		t.Fatal("expected queue onStartFunc to be present for tts_start enqueue")
	}
	item.onStartFunc()
	if startedCount != 0 {
		t.Fatal("expected playback start hook not to run from llm queue onStartFunc")
	}

	hook := ttsPlaybackStartHookFromContext(item.ctx)
	if hook == nil {
		t.Fatal("expected playback start hook to be carried in llm item ctx")
	}

	hook()
	hook()
	if startedCount != 1 {
		t.Fatalf("expected playback start hook to fire once, got %d", startedCount)
	}
}

type speakRequestTestConn struct {
	mu            sync.Mutex
	transportType string
	deviceID      string
	sentCmds      [][]byte
	sendCmdErr    error
	udpSession    *mqtt_udp.UdpSession
	onClose       func(deviceId string)
}

func newSpeakRequestTestManager(transportType string) (*ChatManager, *speakRequestTestConn) {
	ctx, cancel := context.WithCancel(context.Background())
	clientState := &data_client.ClientState{
		Ctx:         ctx,
		Cancel:      cancel,
		Dialogue:    &data_client.Dialogue{},
		DeviceID:    "test-device",
		SessionID:   "test-session",
		ListenPhase: data_client.ListenPhaseIdle,
		Status:      data_client.ClientStatusInit,
	}
	conn := &speakRequestTestConn{
		transportType: transportType,
		deviceID:      clientState.DeviceID,
	}
	manager := &ChatManager{
		DeviceID:          clientState.DeviceID,
		transport:         conn,
		clientState:       clientState,
		serverTransport:   NewServerTransport(conn, clientState),
		ctx:               ctx,
		cancel:            cancel,
		speakReadyTimeout: 200 * time.Millisecond,
	}
	return manager, conn
}

func (c *speakRequestTestConn) SendCmd(msg []byte) error {
	if c.sendCmdErr != nil {
		return c.sendCmdErr
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	msgCopy := make([]byte, len(msg))
	copy(msgCopy, msg)
	c.sentCmds = append(c.sentCmds, msgCopy)
	return nil
}

func (c *speakRequestTestConn) RecvCmd(ctx context.Context, timeout int) ([]byte, error) {
	return nil, context.DeadlineExceeded
}

func (c *speakRequestTestConn) SendAudio(audio []byte) error {
	return nil
}

func (c *speakRequestTestConn) RecvAudio(ctx context.Context, timeout int) ([]byte, error) {
	return nil, context.DeadlineExceeded
}

func (c *speakRequestTestConn) GetDeviceID() string {
	return c.deviceID
}

func (c *speakRequestTestConn) Close() error {
	if c.onClose != nil {
		c.onClose(c.deviceID)
	}
	return nil
}

func (c *speakRequestTestConn) OnClose(handler func(deviceId string)) {
	c.onClose = handler
}

func (c *speakRequestTestConn) CloseAudioChannel() error {
	return nil
}

func (c *speakRequestTestConn) GetTransportType() string {
	return c.transportType
}

func (c *speakRequestTestConn) GetData(key string) (interface{}, error) {
	return nil, nil
}

func (c *speakRequestTestConn) GetUdpSession() *mqtt_udp.UdpSession {
	return c.udpSession
}

func (c *speakRequestTestConn) sentCmdCount() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.sentCmds)
}

func waitForServerMessage(t *testing.T, conn *speakRequestTestConn, index int) msgdata.ServerMessage {
	t.Helper()

	deadline := time.Now().Add(time.Second)
	for time.Now().Before(deadline) {
		conn.mu.Lock()
		if len(conn.sentCmds) > index {
			payload := make([]byte, len(conn.sentCmds[index]))
			copy(payload, conn.sentCmds[index])
			conn.mu.Unlock()

			var serverMsg msgdata.ServerMessage
			if err := json.Unmarshal(payload, &serverMsg); err != nil {
				t.Fatalf("failed to decode server message: %v", err)
			}
			return serverMsg
		}
		conn.mu.Unlock()
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatalf("timed out waiting for server message %d", index)
	return msgdata.ServerMessage{}
}
