package chat

import "context"

func (s *ChatSession) StopSpeaking(isSendTtsStop bool) {
	s.clientState.SessionCtx.Cancel()
	s.clientState.AfterAsrSessionCtx.Cancel()

	s.ClearChatTextQueue()
	s.llmManager.ClearLLMResponseQueue()
	s.ttsManager.ClearTTSQueue()
	s.ttsManager.InterruptAndStop(s.clientState.Ctx, isSendTtsStop, context.Canceled)

}

func (s *ChatSession) MqttClose() {
	s.serverTransport.SendMqttGoodbye()
}
