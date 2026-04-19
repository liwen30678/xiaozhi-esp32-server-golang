package client

import "testing"

func TestShouldCountAudioIdleTimeoutRealtimeOutputStates(t *testing.T) {
	state := &ClientState{ListenMode: "realtime"}

	if !state.ShouldCountAudioIdleTimeout() {
		t.Fatal("expected realtime idle timeout to count before assistant output starts")
	}

	state.SetStatus(ClientStatusLLMStart)
	if state.ShouldCountAudioIdleTimeout() {
		t.Fatal("expected realtime idle timeout to pause during LLM output")
	}

	state.SetStatus(ClientStatusTTSStart)
	if state.ShouldCountAudioIdleTimeout() {
		t.Fatal("expected realtime idle timeout to pause during TTS output")
	}

	state.SetStatus(ClientStatusListenStop)
	state.SetTtsStart(true)
	if state.ShouldCountAudioIdleTimeout() {
		t.Fatal("expected realtime idle timeout to pause while TTS start flag is active")
	}

	state.SetTtsStart(false)
	if !state.ShouldCountAudioIdleTimeout() {
		t.Fatal("expected realtime idle timeout to resume after TTS stop")
	}
}

func TestShouldCountAudioIdleTimeoutNonRealtimeKeepsExistingBehavior(t *testing.T) {
	state := &ClientState{
		ListenMode: "auto",
		Status:     ClientStatusTTSStart,
	}
	state.SetTtsStart(true)

	if !state.ShouldCountAudioIdleTimeout() {
		t.Fatal("expected non-realtime idle timeout behavior to stay unchanged")
	}
}
