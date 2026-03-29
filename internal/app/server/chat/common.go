package chat

import (
	"context"
	"errors"
	"time"

	log "xiaozhi-esp32-server-golang/logger"
)

const stopSpeakingInterruptTimeout = 2 * time.Second

func (s *ChatSession) StopSpeaking(isSendTtsStop bool) {
	s.clientState.SessionCtx.Cancel()
	s.clientState.AfterAsrSessionCtx.Cancel()
	s.clientState.IsWelcomePlaying = false
	s.invalidateListenStart()

	s.ClearChatTextQueue()
	s.llmManager.ClearLLMResponseQueue()
	s.ttsManager.ClearTTSQueue()
	interruptCtx, cancel := context.WithTimeout(context.Background(), stopSpeakingInterruptTimeout)
	defer cancel()
	if err := s.ttsManager.InterruptAndClearQueueSync(interruptCtx); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Warnf("StopSpeaking sync interrupt timed out")
		} else if !errors.Is(err, context.Canceled) {
			log.Warnf("StopSpeaking sync interrupt failed: %v", err)
		}
	}

	if isSendTtsStop {
		s.serverTransport.SendTtsStop()
	}

}

func (s *ChatSession) MqttClose() {
	s.serverTransport.SendMqttGoodbye()
}
