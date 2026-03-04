package chat

func (s *ChatSession) StopSpeaking(isSendTtsStop bool) {
	s.clientState.SessionCtx.Cancel()
	s.clientState.AfterAsrSessionCtx.Cancel()

	s.ClearChatTextQueue()
	s.llmManager.ClearLLMResponseQueue()
	s.ttsManager.ClearTTSQueue()
	s.ttsManager.InterruptAndClearQueue()

	if isSendTtsStop {
		s.serverTransport.SendTtsStop()
	}

}

func (s *ChatSession) MqttClose() {
	s.serverTransport.SendMqttGoodbye()
}
