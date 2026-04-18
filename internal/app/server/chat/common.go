package chat

import (
	"context"
	"errors"
	"time"

	log "xiaozhi-esp32-server-golang/logger"
)

const stopSpeakingInterruptTimeout = 2 * time.Second

func (s *ChatSession) StopSpeaking(isSendTtsStop bool) {
	s.stopSpeakingWithLock(true, isSendTtsStop, true)
}

func (s *ChatSession) StopSpeakingAfterAsr(isSendTtsStop bool) {
	s.stopSpeakingWithLock(false, isSendTtsStop, true)
}

// stopSpeakingWithLock 在 stopSpeaking 基础上增加了 mutex 保护
func (s *ChatSession) stopSpeakingWithLock(cancelSession bool, isSendTtsStop bool, suspendMedia bool) {
	s.stopSpeakingMu.Lock()
	defer s.stopSpeakingMu.Unlock()
	s.stopSpeaking(cancelSession, isSendTtsStop, suspendMedia)
}

func (s *ChatSession) stopSpeaking(cancelSession bool, isSendTtsStop bool, suspendMedia bool) {
	if cancelSession {
		s.clientState.SessionCtx.CancelWithReason("ChatSession.stopSpeaking: session_ctx")
		s.invalidateListenStart()
	}
	s.clientState.AfterAsrSessionCtx.CancelWithReason("ChatSession.stopSpeaking: after_asr_ctx")

	if suspendMedia && s.mediaPlayer != nil {
		if err := s.mediaPlayer.Suspend(); err != nil && !errors.Is(err, context.Canceled) {
			log.Warnf("stopSpeaking 挂起媒体播放失败: %v", err)
		}
	}

	s.clientState.IsWelcomePlaying = false

	s.ClearChatTextQueue()
	s.llmManager.ClearLLMResponseQueue()
	s.ttsManager.ClearTTSQueue()
	interruptCtx, cancel := context.WithTimeout(context.Background(), stopSpeakingInterruptTimeout)
	defer cancel()
	if err := s.ttsManager.InterruptAndStopSync(interruptCtx, isSendTtsStop, context.Canceled); err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			log.Warnf("stopSpeaking sync interrupt timed out, cancel_session=%v", cancelSession)
		} else if !errors.Is(err, context.Canceled) {
			log.Warnf("stopSpeaking sync interrupt failed, cancel_session=%v, err=%v", cancelSession, err)
		}
	}
}

func (s *ChatSession) MqttClose() {
	s.serverTransport.SendMqttGoodbye()
}

func (s *ChatSession) ResetToSilentState() {
	if s == nil || s.IsClosing() {
		return
	}

	s.cancelPendingDetectLLM()
	s.finishOpenClawWarmup("", false)
	s.clearOpenClawStreams()
	s.clearPendingSpeakerResult()

	if s.clientState != nil {
		s.clientState.Abort = false
		s.clientState.IsWelcomeSpeaking = false
	}

	s.stopSpeakingWithLock(true, true, true)

	if s.clientState != nil {
		s.clientState.Destroy()
		s.clientState.Abort = false
		s.clientState.IsWelcomeSpeaking = false
		s.clientState.IsWelcomePlaying = false
	}
}
