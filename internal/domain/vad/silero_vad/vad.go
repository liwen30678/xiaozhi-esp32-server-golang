package silero_vad

import (
	"errors"
	"sync"
	log "xiaozhi-esp32-server-golang/logger"

	. "xiaozhi-esp32-server-golang/internal/domain/vad/inter"

	"github.com/streamer45/silero-vad-go/speech"
)

// VAD默认配置
var defaultVADConfig = map[string]interface{}{
	"threshold":               0.5,
	"min_silence_duration_ms": int64(100),
	"sample_rate":             16000,
	"channels":                1,
	"speech_pad_ms":           60,
}

// 全局变量和初始化
var (
	// 全局解码器实例池
	opusDecoderMap sync.Map
	// 全局VAD检测器实例池
	vadDetectorMap sync.Map
	// 全局初始化锁
	initMutex sync.Mutex
	// 初始化标志
	initialized = false
)

// SileroVAD Silero VAD模型实现
type SileroVAD struct {
	detector         *speech.Detector
	vadThreshold     float32
	silenceThreshold int64 // 单位:毫秒
	sampleRate       int   // 采样率
	channels         int   // 通道数
	mu               sync.Mutex
}

// NewSileroVAD 创建SileroVAD实例
func NewSileroVAD(config map[string]interface{}) (*SileroVAD, error) {
	threshold, ok := config["threshold"].(float64)
	if !ok {
		threshold = 0.5 // 默认阈值
	}

	silenceMs, ok := config["min_silence_duration_ms"].(int64)
	if !ok {
		silenceMs = 800 // 默认500毫秒
	}

	sampleRate, ok := config["sample_rate"].(int)
	if !ok {
		sampleRate = 16000 // 默认采样率
	}

	channels, ok := config["channels"].(int)
	if !ok {
		channels = 1 // 默认单声道
	}

	speechPadMs, ok := config["speech_pad_ms"].(int)
	if !ok {
		speechPadMs = 30 // 默认语音前后填充
	}

	modelPath, ok := config["model_path"].(string)
	if !ok {
		return nil, errors.New("缺少模型路径配置")
	}

	// 创建语音检测器
	detector, err := speech.NewDetector(speech.DetectorConfig{
		ModelPath:            modelPath,
		SampleRate:           sampleRate,
		Threshold:            float32(threshold),
		MinSilenceDurationMs: int(silenceMs),
		SpeechPadMs:          speechPadMs,
		LogLevel:             speech.LogLevelWarn,
	})
	if err != nil {
		return nil, err
	}

	return &SileroVAD{
		detector:         detector,
		vadThreshold:     float32(threshold),
		silenceThreshold: silenceMs,
		sampleRate:       sampleRate,
		channels:         channels,
	}, nil
}

func (s *SileroVAD) IsVADExt(pcmData []float32, sampleRate int, frameSize int) (bool, error) {
	return s.IsVAD(pcmData)
}

// IsVAD 实现VAD接口的IsVAD方法
func (s *SileroVAD) IsVAD(pcmData []float32) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	segments, err := s.detector.Detect(pcmData)
	if err != nil {
		log.Errorf("检测失败: %s", err)
		return false, err
	}

	for _, s := range segments {
		log.Debugf("speech starts at %0.2fs", s.SpeechStartAt)
		if s.SpeechEndAt > 0 {
			log.Debugf("speech ends at %0.2fs", s.SpeechEndAt)
		}
	}

	return len(segments) > 0, nil
}

// Close 关闭并释放资源
func (s *SileroVAD) Close() error {
	if s.detector != nil {
		return s.detector.Destroy()
	}
	return nil
}

// IsValid 检查资源是否有效
func (s *SileroVAD) IsValid() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.detector != nil
}

// AcquireVAD 创建并返回 Silero VAD 实例（由全局资源池管理）
func AcquireVAD(config map[string]interface{}) (VAD, error) {
	return NewSileroVAD(config)
}

// ReleaseVAD 释放 VAD 实例
func ReleaseVAD(vad VAD) error {
	if vad != nil {
		return vad.Close()
	}
	return nil
}

// Reset 重置VAD检测器状态
func (s *SileroVAD) Reset() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	return s.detector.Reset()
}

// SetThreshold 设置VAD检测阈值
func (s *SileroVAD) SetThreshold(threshold float32) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.vadThreshold = threshold
	// 注意：silero-vad-go 库的 detector 没有直接提供 SetThreshold 方法
	// 只能修改实例的阈值，在下次检测时生效
}
