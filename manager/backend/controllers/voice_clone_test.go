package controllers

import "testing"

func TestPickVoiceID(t *testing.T) {
	tests := []struct {
		name string
		in   map[string]any
		want string
	}{
		{name: "top-level voice id", in: map[string]any{"voice_id": "vc_1"}, want: "vc_1"},
		{name: "nested voice id", in: map[string]any{"data": map[string]any{"voiceId": "vc_2"}}, want: "vc_2"},
		{name: "speaker id fallback", in: map[string]any{"speaker_id": "spk_1"}, want: "spk_1"},
		{name: "missing", in: map[string]any{"data": map[string]any{"x": "y"}}, want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := pickVoiceID(tt.in); got != tt.want {
				t.Fatalf("pickVoiceID() = %q, want %q", got, tt.want)
			}
		})
	}
}
