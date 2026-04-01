package chat

import (
	"context"
	"errors"
	"time"

	log "xiaozhi-esp32-server-golang/logger"
)

const stopSpeakingInterruptTimeout = 2 * time.Second

func (s *ChatSession) StopSpeaking(isSendTtsStop bool) {
	s.stopSpeakingWithLock(true, isSendTtsStop)
}

func (s *ChatSession) StopSpeakingAfterAsr(isSendTtsStop bool) {
	s.stopSpeakingWithLock(false, isSendTtsStop)
}

// stopSpeakingWithLock 在 stopSpeaking 基础上增加了 mutex 保护
func (s *ChatSession) stopSpeakingWithLock(cancelSession bool, isSendTtsStop bool) {
	s.stopSpeakingMu.Lock()
	defer s.stopSpeakingMu.Unlock()
	s.stopSpeaking(cancelSession, isSendTtsStop)
}

func (s *ChatSession) stopSpeaking(cancelSession bool, isSendTtsStop bool) {
	if cancelSession {
		s.clientState.SessionCtx.CancelWithReason("ChatSession.stopSpeaking: session_ctx")
		s.invalidateListenStart()
	}
	s.clientState.AfterAsrSessionCtx.CancelWithReason("ChatSession.stopSpeaking: after_asr_ctx")

	s.clientState.IsWelcomePlaying = false

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
