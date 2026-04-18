package chat

import (
	"context"
	"testing"
	"time"

	data_audio "xiaozhi-esp32-server-golang/internal/data/audio"
	data_client "xiaozhi-esp32-server-golang/internal/data/client"
	msgdata "xiaozhi-esp32-server-golang/internal/data/msg"
	config_types "xiaozhi-esp32-server-golang/internal/domain/config/types"
	llm_common "xiaozhi-esp32-server-golang/internal/domain/llm/common"
)

func TestClearTTSQueueDismissesDrainedItemsForTurnBarrier(t *testing.T) {
	manager := newTestTTSManager(false)
	ctx := ensureTTSTurnTrackerInContext(context.Background())
	tracker := ttsTurnTrackerFromContext(ctx)
	if tracker == nil {
		t.Fatal("expected tracker to be stored in context")
	}

	if err := manager.handleTextResponseWithHooks(ctx, llm_common.LLMResponseStruct{Text: "你好"}, false, tracker.Add, nil); err != nil {
		t.Fatalf("enqueue tts item failed: %v", err)
	}

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- waitForTTSTurnDrainIfRoot(ctx)
	}()

	select {
	case err := <-waitDone:
		t.Fatalf("turn barrier returned before clear dismissed queued item: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	manager.ClearTTSQueue()

	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("turn barrier returned unexpected error after clear: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("turn barrier did not finish after ClearTTSQueue")
	}
}

func TestClearTTSQueueResetsDualStreamState(t *testing.T) {
	manager := newTestTTSManager(true)
	ctx := ensureTTSTurnTrackerInContext(context.Background())
	tracker := ttsTurnTrackerFromContext(ctx)
	if tracker == nil {
		t.Fatal("expected tracker to be stored in context")
	}

	if err := manager.handleTextResponseWithHooks(ctx, llm_common.LLMResponseStruct{
		Text:    "第一段",
		IsStart: true,
	}, false, tracker.Add, nil); err != nil {
		t.Fatalf("enqueue dual-stream tts item failed: %v", err)
	}

	manager.dualStreamMu.Lock()
	if manager.dualStreamChan == nil {
		manager.dualStreamMu.Unlock()
		t.Fatal("expected dualStreamChan to be active before clear")
	}
	manager.dualStreamMu.Unlock()

	waitDone := make(chan error, 1)
	go func() {
		waitDone <- waitForTTSTurnDrainIfRoot(ctx)
	}()

	select {
	case err := <-waitDone:
		t.Fatalf("turn barrier returned before clear dismissed dual-stream item: %v", err)
	case <-time.After(20 * time.Millisecond):
	}

	manager.ClearTTSQueue()

	manager.dualStreamMu.Lock()
	if manager.dualStreamChan != nil {
		manager.dualStreamMu.Unlock()
		t.Fatal("expected dualStreamChan to be cleared")
	}
	if manager.dualStreamDone != nil {
		manager.dualStreamMu.Unlock()
		t.Fatal("expected dualStreamDone to be cleared")
	}
	manager.dualStreamMu.Unlock()

	select {
	case err := <-waitDone:
		if err != nil {
			t.Fatalf("turn barrier returned unexpected error after dual-stream clear: %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("turn barrier did not finish after dual-stream clear")
	}
}

func TestBeginExclusiveMediaPlaybackDoesNotFinishTtsStopImmediately(t *testing.T) {
	manager := newTestTTSManager(false)
	manager.ttsActive.Store(true)

	if err := manager.BeginExclusiveMediaPlayback(context.Background()); err != nil {
		t.Fatalf("BeginExclusiveMediaPlayback returned error: %v", err)
	}

	if !manager.ttsActive.Load() {
		t.Fatal("expected ttsActive to remain true until outer turn cleanup sends tts_stop")
	}

	manager.mediaPlaybackMu.RLock()
	active := manager.mediaPlaybackActive
	waitCh := manager.mediaPlaybackWaitCh
	manager.mediaPlaybackMu.RUnlock()

	if !active {
		t.Fatal("expected media playback to be marked active")
	}
	if waitCh == nil {
		t.Fatal("expected media playback wait channel to be created")
	}

	manager.EndExclusiveMediaPlayback()
}

func TestHandleLLMResponseChannelSyncSkipsTtsCommandsWhenContextAlreadyCanceled(t *testing.T) {
	session, conn, cleanup := newStartedTTSControlTestSession(t)
	defer cleanup()

	turnCtx, cancel := context.WithCancel(context.Background())
	cancel()

	responseChan := make(chan llm_common.LLMResponseStruct)
	close(responseChan)

	if _, err := session.llmManager.HandleLLMResponseChannelSync(turnCtx, nil, responseChan, nil); err != nil {
		t.Fatalf("HandleLLMResponseChannelSync returned error: %v", err)
	}

	time.Sleep(50 * time.Millisecond)
	if count := conn.sentCmdCount(); count != 0 {
		t.Fatalf("expected canceled ctx to skip all tts commands, got %d", count)
	}
}

func TestStopSpeakingSendsTtsStopForActiveTTS(t *testing.T) {
	session, conn, cleanup := newStartedTTSControlTestSession(t)
	defer cleanup()

	session.ttsManager.EnqueueTtsStart(context.Background())

	startMsg := waitForServerMessage(t, conn, 0)
	if startMsg.Type != msgdata.ServerMessageTypeTts || startMsg.State != msgdata.MessageStateStart {
		t.Fatalf("expected first server message to be tts start, got type=%s state=%s", startMsg.Type, startMsg.State)
	}

	session.StopSpeaking(true)

	stopMsg := waitForServerMessage(t, conn, 1)
	if stopMsg.Type != msgdata.ServerMessageTypeTts || stopMsg.State != msgdata.MessageStateStop {
		t.Fatalf("expected second server message to be tts stop, got type=%s state=%s", stopMsg.Type, stopMsg.State)
	}
}

func newTestTTSManager(dualStream bool) *TTSManager {
	ttsConfig := map[string]interface{}{}
	if dualStream {
		ttsConfig["double_stream"] = true
	}

	return NewTTSManager(&data_client.ClientState{
		Ctx:      context.Background(),
		Dialogue: &data_client.Dialogue{},
		DeviceConfig: config_types.UConfig{
			Tts: config_types.TtsConfig{
				Config: ttsConfig,
			},
		},
		OutputAudioFormat: data_audio.AudioFormat{
			SampleRate:    data_audio.SampleRate,
			Channels:      data_audio.Channels,
			FrameDuration: data_audio.FrameDuration,
		},
	}, nil, nil)
}

func newStartedTTSControlTestSession(t *testing.T) (*ChatSession, *speakRequestTestConn, func()) {
	t.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	clientState := &data_client.ClientState{
		Ctx:         ctx,
		Cancel:      cancel,
		Dialogue:    &data_client.Dialogue{},
		DeviceID:    "tts-control-device",
		SessionID:   "tts-control-session",
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

	ttsDone := make(chan struct{})
	go func() {
		session.ttsManager.Start(ctx)
		close(ttsDone)
	}()

	cleanup := func() {
		cancel()
		select {
		case <-ttsDone:
		case <-time.After(time.Second):
			t.Fatal("timed out waiting for tts manager to stop")
		}
	}

	return session, conn, cleanup
}
