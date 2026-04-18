package chat

import (
	"context"
	"testing"
	"time"

	data_audio "xiaozhi-esp32-server-golang/internal/data/audio"
	data_client "xiaozhi-esp32-server-golang/internal/data/client"
	msgdata "xiaozhi-esp32-server-golang/internal/data/msg"
	"xiaozhi-esp32-server-golang/internal/util"

	"github.com/spf13/viper"
)

func TestIsWithinCommandTTLUses800msWindow(t *testing.T) {
	now := time.Now()

	if !isWithinCommandTTL(now.Add(-800*time.Millisecond), now) {
		t.Fatal("expected 800ms-old command to stay within TTL")
	}
	if isWithinCommandTTL(now.Add(-801*time.Millisecond), now) {
		t.Fatal("expected command older than 800ms to fall outside TTL")
	}
}

func TestHandleListenDetectDebouncesLLMQueue(t *testing.T) {
	session := newDetectDebounceTestSession(t)
	setViperValueForTest(t, "enable_greeting", false)

	if err := session.HandleListenDetect(&data_client.ClientMessage{
		Type:     msgdata.MessageTypeListen,
		DeviceID: session.clientState.DeviceID,
		Text:     "普通问题",
	}); err != nil {
		t.Fatalf("HandleListenDetect returned error: %v", err)
	}

	if _, err := session.chatTextQueue.Pop(context.Background(), 200*time.Millisecond); err != util.ErrQueueTimeout {
		t.Fatalf("expected debounce to delay queue push, got %v", err)
	}

	item, err := session.chatTextQueue.Pop(context.Background(), 500*time.Millisecond)
	if err != nil {
		t.Fatalf("expected debounced detect llm to be enqueued, got %v", err)
	}
	if item.text != "普通问题" {
		t.Fatalf("expected debounced text to be preserved, got %q", item.text)
	}
}

func TestHandleListenStartCancelsPendingDetectLLM(t *testing.T) {
	session := newDetectDebounceTestSession(t)
	setViperValueForTest(t, "enable_greeting", false)

	if err := session.HandleListenDetect(&data_client.ClientMessage{
		Type:     msgdata.MessageTypeListen,
		DeviceID: session.clientState.DeviceID,
		Text:     "普通问题",
	}); err != nil {
		t.Fatalf("HandleListenDetect returned error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)
	session.clientState.SetListenPhase(data_client.ListenPhaseStarting)

	if err := session.HandleListenStart(&data_client.ClientMessage{
		Type:     msgdata.MessageTypeListen,
		DeviceID: session.clientState.DeviceID,
		Mode:     "auto",
	}); err != nil {
		t.Fatalf("HandleListenStart returned error: %v", err)
	}

	if _, err := session.chatTextQueue.Pop(context.Background(), 500*time.Millisecond); err != util.ErrQueueTimeout {
		t.Fatalf("expected listen start to cancel pending detect llm, got %v", err)
	}
}

func newDetectDebounceTestSession(t *testing.T) *ChatSession {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	clientState := &data_client.ClientState{
		Ctx:         ctx,
		Cancel:      cancel,
		Dialogue:    &data_client.Dialogue{},
		DeviceID:    "detect-test-device",
		SessionID:   "detect-test-session",
		ListenPhase: data_client.ListenPhaseIdle,
		Status:      data_client.ClientStatusInit,
		OutputAudioFormat: data_audio.AudioFormat{
			SampleRate:    data_audio.SampleRate,
			Channels:      data_audio.Channels,
			FrameDuration: data_audio.FrameDuration,
		},
	}

	conn := &speakRequestTestConn{
		transportType: "websocket",
		deviceID:      clientState.DeviceID,
	}
	session := NewChatSession(clientState, NewServerTransport(conn, clientState), nil, nil)

	t.Cleanup(func() {
		session.cancelPendingDetectLLM()
		cancel()
	})

	return session
}

func setViperValueForTest(t *testing.T, key string, value any) {
	t.Helper()

	oldValue := viper.Get(key)
	viper.Set(key, value)
	t.Cleanup(func() {
		viper.Set(key, oldValue)
	})
}
