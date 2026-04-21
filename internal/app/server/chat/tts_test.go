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

func TestRealtimeStopSpeakingSendsTtsStopForActiveTTS(t *testing.T) {
	session, conn, cleanup := newStartedTTSControlTestSession(t)
	defer cleanup()
	session.clientState.ListenMode = "realtime"

	session.ttsManager.EnqueueTtsStart(context.Background())

	startMsg := waitForServerMessage(t, conn, 0)
	if startMsg.Type != msgdata.ServerMessageTypeTts || startMsg.State != msgdata.MessageStateStart {
		t.Fatalf("expected first server message to be tts start, got type=%s state=%s", startMsg.Type, startMsg.State)
	}

	session.StopSpeaking(true)

	stopMsg := waitForServerMessage(t, conn, 1)
	if stopMsg.Type != msgdata.ServerMessageTypeTts || stopMsg.State != msgdata.MessageStateStop {
		t.Fatalf("expected realtime tts stop after interrupt, got type=%s state=%s", stopMsg.Type, stopMsg.State)
	}
	if session.clientState.GetTtsStart() {
		t.Fatal("expected realtime tts stop to clear TTS start flag")
	}
}

func TestRealtimeLLMNaturalEndEnqueuesTtsStop(t *testing.T) {
	session, conn, cleanup := newStartedTTSControlTestSession(t)
	defer cleanup()
	session.clientState.ListenMode = "realtime"

	responseChan := make(chan llm_common.LLMResponseStruct)
	close(responseChan)

	if _, err := session.llmManager.HandleLLMResponseChannelSync(context.Background(), nil, responseChan, nil); err != nil {
		t.Fatalf("HandleLLMResponseChannelSync returned error: %v", err)
	}

	startMsg := waitForServerMessage(t, conn, 0)
	if startMsg.Type != msgdata.ServerMessageTypeTts || startMsg.State != msgdata.MessageStateStart {
		t.Fatalf("expected first server message to be tts start, got type=%s state=%s", startMsg.Type, startMsg.State)
	}

	stopMsg := waitForServerMessage(t, conn, 1)
	if stopMsg.Type != msgdata.ServerMessageTypeTts || stopMsg.State != msgdata.MessageStateStop {
		t.Fatalf("expected realtime natural end to send tts stop, got type=%s state=%s", stopMsg.Type, stopMsg.State)
	}
	if session.clientState.GetStatus() != data_client.ClientStatusListenStop {
		t.Fatalf("expected realtime tts stop to return status to listenStop, got %s", session.clientState.GetStatus())
	}
}

func TestFinishTtsWithoutProtocolStopClearsLocalTTSState(t *testing.T) {
	manager := newTestTTSManager(false)
	manager.clientState.ListenMode = "realtime"
	manager.clientState.SetStatus(data_client.ClientStatusTTSStart)
	manager.clientState.SetTtsStart(true)
	manager.ttsActive.Store(true)
	manager.clientState.StartAudioIdleWindow(time.Now().Add(-5 * time.Second))
	manager.clientState.PauseAudioIdleWindow(time.Now().Add(-4 * time.Second))

	if !manager.FinishTtsWithoutProtocolStop(context.Background(), nil) {
		t.Fatal("expected active TTS turn to finish")
	}
	if manager.clientState.GetTtsStart() {
		t.Fatal("expected logical TTS stop to clear local TTS start flag")
	}
	if manager.clientState.GetStatus() != data_client.ClientStatusListenStop {
		t.Fatalf("expected logical TTS stop to return status to listenStop, got %s", manager.clientState.GetStatus())
	}
	if !manager.clientState.AudioIdleStarted() {
		t.Fatal("expected realtime logical tts stop to restart audio idle window")
	}
	if manager.clientState.AudioIdlePaused() {
		t.Fatal("expected realtime logical tts stop to resume idle counting")
	}
	if elapsed := manager.clientState.GetAudioIdleElapsed(time.Now()); elapsed > time.Second {
		t.Fatalf("expected realtime logical tts stop to reset idle window, got elapsed=%s", elapsed)
	}
}

func TestFinishTtsWithoutProtocolStopRestartsAutoIdleWindow(t *testing.T) {
	manager := newTestTTSManager(false)
	manager.clientState.ListenMode = "auto"
	manager.clientState.SetStatus(data_client.ClientStatusTTSStart)
	manager.clientState.SetTtsStart(true)
	manager.ttsActive.Store(true)
	manager.clientState.StartAudioIdleWindow(time.Now().Add(-5 * time.Second))
	manager.clientState.PauseAudioIdleWindow(time.Now().Add(-4 * time.Second))
	manager.clientState.SetClientVoiceStop(true)

	if !manager.FinishTtsWithoutProtocolStop(context.Background(), nil) {
		t.Fatal("expected active auto TTS turn to finish")
	}
	if !manager.clientState.AudioIdleStarted() {
		t.Fatal("expected auto logical tts stop to restart audio idle window")
	}
	if manager.clientState.AudioIdlePaused() {
		t.Fatal("expected auto logical tts stop to resume idle counting")
	}
	if manager.clientState.GetClientVoiceStop() {
		t.Fatal("expected auto logical tts stop to clear voice stop flag")
	}
	if elapsed := manager.clientState.GetAudioIdleElapsed(time.Now()); elapsed > time.Second {
		t.Fatalf("expected auto logical tts stop to reset idle window, got elapsed=%s", elapsed)
	}
}

func TestRealtimeInactiveTTSStopClearsInterruptedLLMState(t *testing.T) {
	manager := newTestTTSManager(false)
	manager.clientState.ListenMode = "realtime"
	manager.clientState.SetStatus(data_client.ClientStatusLLMStart)
	manager.clientState.SetTtsStart(false)
	manager.clientState.StartAudioIdleWindow(time.Now().Add(-6 * time.Second))
	manager.clientState.PauseAudioIdleWindow(time.Now().Add(-5 * time.Second))

	if manager.finishTtsStop(context.Background(), true, context.Canceled) {
		t.Fatal("expected inactive TTS stop to report no active TTS turn")
	}
	if manager.clientState.GetStatus() != data_client.ClientStatusListenStop {
		t.Fatalf("expected interrupted realtime LLM state to return to listenStop, got %s", manager.clientState.GetStatus())
	}
	if manager.clientState.GetTtsStart() {
		t.Fatal("expected interrupted realtime LLM state to keep TTS start flag cleared")
	}
	if !manager.clientState.AudioIdleStarted() {
		t.Fatal("expected interrupted realtime LLM state to restart audio idle window")
	}
	if manager.clientState.AudioIdlePaused() {
		t.Fatal("expected interrupted realtime LLM state to resume idle counting")
	}
	if elapsed := manager.clientState.GetAudioIdleElapsed(time.Now()); elapsed > time.Second {
		t.Fatalf("expected interrupted realtime LLM state to reset idle window, got elapsed=%s", elapsed)
	}
}

func TestFinishTtsStopKeepsRealtimeListenSessionActive(t *testing.T) {
	session, _, cleanup := newStartedTTSControlTestSession(t)
	defer cleanup()

	session.realtimeListenSessionActive.Store(true)
	session.ttsManager.ttsActive.Store(true)

	if !session.ttsManager.finishTtsStop(context.Background(), false, nil) {
		t.Fatal("expected active TTS turn to finish")
	}
	if !session.isRealtimeListenSessionActive() {
		t.Fatal("expected tts stop to keep realtime listen session active")
	}
}

func TestInactiveAutoTTSStopRestartsIdleWindow(t *testing.T) {
	manager := newTestTTSManager(false)
	manager.clientState.ListenMode = "auto"
	manager.clientState.SetStatus(data_client.ClientStatusListenStop)
	manager.clientState.SetTtsStart(false)
	manager.clientState.StartAudioIdleWindow(time.Now().Add(-6 * time.Second))
	manager.clientState.PauseAudioIdleWindow(time.Now().Add(-5 * time.Second))
	manager.clientState.SetClientVoiceStop(true)

	if manager.finishTtsStop(context.Background(), false, context.Canceled) {
		t.Fatal("expected inactive auto TTS stop to report no active TTS turn")
	}
	if !manager.clientState.AudioIdleStarted() {
		t.Fatal("expected inactive auto TTS stop to restart audio idle window")
	}
	if manager.clientState.AudioIdlePaused() {
		t.Fatal("expected inactive auto TTS stop to resume idle counting")
	}
	if manager.clientState.GetClientVoiceStop() {
		t.Fatal("expected inactive auto TTS stop to clear voice stop flag")
	}
	if elapsed := manager.clientState.GetAudioIdleElapsed(time.Now()); elapsed > time.Second {
		t.Fatalf("expected inactive auto TTS stop to reset idle window, got elapsed=%s", elapsed)
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
