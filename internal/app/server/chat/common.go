package chat

import (
	"context"
	"errors"
	"time"

	log "xiaozhi-esp32-server-golang/logger"
)

const stopSpeakingInterruptTimeout = 2 * time.Second

func (s *ChatSession) StopSpeaking(isSendTtsStop bool) {
	s.clientState.SessionCtx.CancelWithReason("ChatSession.StopSpeaking: session_ctx")
	s.clientState.AfterAsrSessionCtx.CancelWithReason("ChatSession.StopSpeaking: after_asr_ctx")
	s.clientState.IsWelcomePlaying = false
	s.invalidateListenStart()

	s.ClearChatTextQueue()
	s.llmManager.ClearLLMResponseQueue()
	s.ttsManager.ClearTTSQueue()
	interruptCtx, cancel := context.WithTimeout(context.Background(), stopSpeakingInterruptTimeout)
	defer cancel()
	if err := s.ttsManager.InterruptAndStopSync(interruptCtx, isSendTtsStop, context.Canceled); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Warnf("StopSpeaking sync interrupt timed out")
		} else if !errors.Is(err, context.Canceled) {
			log.Warnf("StopSpeaking sync interrupt failed: %v", err)
		}
	}

}

func (s *ChatSession) MqttClose() {
	s.serverTransport.SendMqttGoodbye()
}
