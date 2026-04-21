package chat

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/schema"
	"github.com/spf13/viper"

	. "xiaozhi-esp32-server-golang/internal/data/client"
	"xiaozhi-esp32-server-golang/internal/data/history"
	. "xiaozhi-esp32-server-golang/internal/data/msg"
	chathooks "xiaozhi-esp32-server-golang/internal/domain/chat/hooks"
	"xiaozhi-esp32-server-golang/internal/domain/chat/streamtransform"
	user_config "xiaozhi-esp32-server-golang/internal/domain/config"
	"xiaozhi-esp32-server-golang/internal/domain/config/types"
	"xiaozhi-esp32-server-golang/internal/domain/eventbus"
	"xiaozhi-esp32-server-golang/internal/domain/llm"
	llm_common "xiaozhi-esp32-server-golang/internal/domain/llm/common"
	"xiaozhi-esp32-server-golang/internal/domain/mcp"
	"xiaozhi-esp32-server-golang/internal/domain/memory"
	"xiaozhi-esp32-server-golang/internal/domain/memory/llm_memory"
	"xiaozhi-esp32-server-golang/internal/domain/openclaw"
	"xiaozhi-esp32-server-golang/internal/domain/speaker"
	"xiaozhi-esp32-server-golang/internal/util"
	log "xiaozhi-esp32-server-golang/logger"
)

type AsrResponseChannelItem struct {
	ctx           context.Context
	text          string
	speakerResult *speaker.IdentifyResult
}

const (
	listenCommandHistoryTTL   = 800 * time.Millisecond
	detectLLMDebounceDuration = 300 * time.Millisecond
)

type detectAction string

const (
	detectActionSilent  detectAction = "silent"
	detectActionWelcome detectAction = "welcome"
	detectActionLLM     detectAction = "llm"
)

const (
	chatSessionCloseReasonManagerShutdown     = "manager_shutdown"
	chatSessionCloseReasonExplicitExit        = "explicit_exit"
	chatSessionCloseReasonFatalError          = "fatal_error"
	chatSessionCloseReasonAudioIdleTimeout    = "audio_idle_timeout"
	chatSessionCloseReasonRetainedIdleTimeout = "retained_idle_timeout"
)

type ChatSession struct {
	clientState     *ClientState
	asrManager      *ASRManager
	ttsManager      *TTSManager
	llmManager      *LLMManager
	speakerManager  *SpeakerManager
	mediaPlayer     *SessionMediaPlayer
	serverTransport *ServerTransport

	ctx    context.Context
	cancel context.CancelFunc

	chatTextQueue *util.Queue[AsrResponseChannelItem]

	// 声纹识别结果暂存（带锁保护）
	speakerResultMu        sync.RWMutex
	pendingSpeakerResult   *speaker.IdentifyResult
	speakerResultReady     chan struct{} // 仅用于通知就绪，不传数据
	turnSpeakerInterrupted atomic.Bool

	vadLoopStarted              bool
	listenStartSeq              atomic.Uint64
	realtimeListenSessionActive atomic.Bool

	// 未激活设备高频触发时，短时间内复用最近一次“未激活”判定，避免频繁打接口。
	activationCheckMu     sync.Mutex
	lastActivationFalseAt time.Time

	// Close 保护，防止多次关闭
	closeOnce sync.Once
	closing   atomic.Bool

	// stopSpeaking 保护，防止与 AddAsrResultToQueue/HandleWelcome 并发冲突
	stopSpeakingMu sync.Mutex

	detectLLMDebounceMu    sync.Mutex
	detectLLMDebounceTimer *time.Timer

	openClawStreamMu sync.Mutex
	openClawStreams  map[string]chan llm_common.LLMResponseStruct

	openClawWarmupMu sync.Mutex
	openClawWarmup   *openClawWarmupTask

	hookHub      *chathooks.Hub
	closeHandler func(session *ChatSession, reason string)
}

type ChatSessionOption func(*ChatSession)

func WithChatSessionCloseHandler(handler func(session *ChatSession, reason string)) ChatSessionOption {
	return func(s *ChatSession) {
		s.closeHandler = handler
	}
}

func NewChatSession(clientState *ClientState, serverTransport *ServerTransport, hookHub *chathooks.Hub, transformRegistry *streamtransform.Registry, opts ...ChatSessionOption) *ChatSession {
	s := &ChatSession{
		clientState:        clientState,
		serverTransport:    serverTransport,
		chatTextQueue:      util.NewQueue[AsrResponseChannelItem](10),
		speakerResultReady: make(chan struct{}, 1), // 缓冲为1，避免阻塞
		openClawStreams:    make(map[string]chan llm_common.LLMResponseStruct),
		hookHub:            hookHub,
	}
	for _, opt := range opts {
		opt(s)
	}

	s.asrManager = NewASRManager(clientState, serverTransport)
	s.asrManager.session = s // 设置 session 引用
	s.ttsManager = NewTTSManager(clientState, serverTransport, s)
	s.mediaPlayer = NewSessionMediaPlayer(s)
	s.llmManager = NewLLMManager(clientState, serverTransport, s.ttsManager, s, transformRegistry)

	// 如果启用声纹识别，创建声纹管理器
	if clientState.IsSpeakerEnabled() {
		// 从系统配置（viper）获取声纹服务地址
		baseURL := viper.GetString("voice_identify.base_url")
		if baseURL != "" {
			// 设置服务地址和阈值到配置中
			speakerConfig := map[string]interface{}{
				"base_url": baseURL,
			}
			// 读取阈值配置，如果未配置则使用默认值 0.6
			if viper.IsSet("voice_identify.threshold") {
				threshold := viper.GetFloat64("voice_identify.threshold")
				speakerConfig["threshold"] = threshold
			}

			provider, err := speaker.GetSpeakerProvider(speakerConfig)
			if err != nil {
				log.Warnf("创建声纹识别提供者失败: %v", err)
			} else {
				clientState.SpeakerProvider = provider
				s.speakerManager = NewSpeakerManager(provider)
				log.Debugf("设备 %s 启用声纹识别", clientState.DeviceID)

				// 设置异步获取声纹结果的回调
				clientState.OnVoiceSilenceSpeakerCallback = func(ctx context.Context) {
					log.Debugf("[声纹识别] OnVoiceSilenceSpeakerCallback 被调用, deviceID: %s", clientState.DeviceID)

					// 异步获取声纹结果
					go func() {
						log.Debugf("[声纹识别] 开始异步获取声纹识别结果, deviceID: %s", clientState.DeviceID)

						// 检查 speakerManager 是否激活
						if !s.speakerManager.IsActive() {
							//log.Warnf("[声纹识别] speakerManager 未激活，无法获取识别结果")
							return
						}
						// 清空之前的结果
						s.speakerResultMu.Lock()
						oldResult := s.pendingSpeakerResult
						s.pendingSpeakerResult = nil
						s.speakerResultMu.Unlock()
						if oldResult != nil {
							log.Debugf("[声纹识别] 清空之前的识别结果: identified=%v, speaker_id=%s", oldResult.Identified, oldResult.SpeakerID)
						}

						// 清空就绪通知（非阻塞）
						select {
						case <-s.speakerResultReady:
							log.Debugf("[声纹识别] 清空就绪通知通道")
						default:
							log.Debugf("[声纹识别] 就绪通知通道已为空")
						}

						result, err := s.speakerManager.FinishAndIdentify(ctx)
						if err != nil {
							log.Warnf("[声纹识别] 获取声纹识别结果失败: %v, deviceID: %s", err, clientState.DeviceID)
							// 声纹识别失败不影响主流程，存储 nil 结果
							s.speakerResultMu.Lock()
							s.pendingSpeakerResult = nil
							s.speakerResultMu.Unlock()
							log.Debugf("[声纹识别] 已存储 nil 结果（识别失败）")
						} else if result != nil && result.Identified {
							log.Infof("[声纹识别] 识别到说话人: %s (置信度: %.4f, 阈值: %.4f), deviceID: %s",
								result.SpeakerName, result.Confidence, result.Threshold, clientState.DeviceID)
							log.Debugf("[声纹识别] 识别结果详情: speaker_id=%s, speaker_name=%s, confidence=%.4f, threshold=%.4f",
								result.SpeakerID, result.SpeakerName, result.Confidence, result.Threshold)
							s.speakerResultMu.Lock()
							s.pendingSpeakerResult = result
							s.speakerResultMu.Unlock()
							log.Debugf("[声纹识别] 已存储识别结果（已识别）")
						} else {
							// 未识别到说话人，也存储结果
							if result != nil {
								log.Debugf("[声纹识别] 未识别到说话人: identified=%v, confidence=%.4f, threshold=%.4f, deviceID: %s",
									result.Identified, result.Confidence, result.Threshold, clientState.DeviceID)
							} else {
								log.Debugf("[声纹识别] 识别结果为 nil, deviceID: %s", clientState.DeviceID)
							}
							s.speakerResultMu.Lock()
							s.pendingSpeakerResult = result
							s.speakerResultMu.Unlock()
							log.Debugf("[声纹识别] 已存储识别结果（未识别）")
						}

						// 通知结果就绪
						select {
						case s.speakerResultReady <- struct{}{}:
							log.Debugf("[声纹识别] 已发送结果就绪通知, deviceID: %s", clientState.DeviceID)
						default:
							log.Warnf("[声纹识别] 结果就绪通知通道已满，无法发送通知, deviceID: %s", clientState.DeviceID)
						}
					}()
				}
			}
		}
	}

	// 设置 ASR 首次返回字符的回调
	clientState.OnAsrFirstTextCallback = func(text string, isFinal bool) {
		clientState.Asr.MarkTextReceived()
		clientState.ClearAudioIdleTimeoutPending()
		clientState.PauseAudioIdleWindow(time.Now())
		log.Debugf("ASR首次返回字符: device=%s, text=%s, isFinal=%v", clientState.DeviceID, text, isFinal)
		clientState.MarkAsrFirstText()
		s.TraceAsrFirstText(clientState.Ctx, time.Now().UnixMilli())
		if clientState.IsRealTime() && viper.GetInt("chat.realtime_mode") == 4 {
			if s.isRealtimeMcpAudioGateActive() {
				log.Debugf("设备 %s realtime媒体播放门控激活，跳过ASR首字打断: text=%s", clientState.DeviceID, text)
				return
			}
			clientState.AfterAsrSessionCtx.CancelWithReason("ChatSession.OnAsrFirstTextCallback: realtime_mode=4")
			s.InterruptAndClearTTSQueue()
		}
	}

	return s
}

func (s *ChatSession) Start(pctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(pctx)

	if s.clientState.InputAudioFormat.SampleRate <= 0 || s.clientState.InputAudioFormat.Channels <= 0 {
		return fmt.Errorf("输入音频格式未初始化，请先完成 hello 握手")
	}

	err := s.InitAsrLlmTts()
	if err != nil {
		log.Errorf("初始化ASR/LLM/TTS失败: %v", err)
		return err
	}

	// 异步加载历史消息，不阻塞会话启动
	go func() {
		err := s.initHistoryMessages()
		if err != nil {
			log.Errorf("初始化对话历史失败: %v", err)
		}
	}()

	if !s.vadLoopStarted {
		// Session 级 idle watchdog 需要独立于单次 ASR loop 生命周期存在，
		// 这样 auto 模式在一轮成功结束后仍能继续统计连接空闲时间。
		go s.asrManager.runAudioIdleTimeoutWatchdog(s.ctx)
		s.asrManager.ProcessVadAudio(s.ctx)
		s.vadLoopStarted = true
	}

	go s.processChatText(s.ctx)  //处理 asr后 的对话消息
	go s.llmManager.Start(s.ctx) //处理 llm后 的一系列返回消息
	go s.ttsManager.Start(s.ctx) //处理 tts的 消息队列
	if s.mediaPlayer != nil {
		s.mediaPlayer.AttachSession()
	}

	return nil
}

// 初始化历史对话记录到内存中
func (s *ChatSession) initHistoryMessages() error {
	var historyMessages []*schema.Message
	var err error

	if s.clientState.GetMemoryMode() == MemoryModeNone {
		log.Debugf("设备 %s 记忆模式=none，跳过历史消息加载", s.clientState.DeviceID)
		return nil
	}

	// 根据配置选择数据源（无优先级关系，直接选择）
	useRedis := s.shouldUseRedis()
	useManager := s.shouldUseManager()

	// 验证必要字段：DeviceID 不能为空
	if s.clientState.DeviceID == "" {
		log.Debugf("DeviceID 为空，跳过历史消息加载（可能在 hello 消息之前调用）")
		return nil
	}

	// 根据配置选择数据源（无优先级关系，直接选择）
	if useRedis {
		// 从 Redis 加载
		historyMessages, err = llm_memory.Get().GetMessages(
			s.ctx,
			s.clientState.DeviceID,
			s.clientState.AgentID,
			20)
		if err != nil {
			log.Warnf("从 Redis 加载历史消息失败: %v", err)
			return err
		}
		log.Infof("从 Redis 加载了 %d 条历史消息", len(historyMessages))
	} else if useManager {
		// 从 Manager 加载
		historyMessages, err = s.loadFromManager()
		if err != nil {
			log.Warnf("从 Manager 加载历史消息失败: %v", err)
			return err
		}
		log.Infof("从 Manager 加载了 %d 条历史消息", len(historyMessages))
	} else {
		// 两个数据源都未配置，不加载历史消息
		log.Debugf("Redis 和 Manager 都未配置，跳过历史消息加载")
		return nil
	}

	if len(historyMessages) > 0 {
		s.clientState.InitMessages(historyMessages)
		log.Infof("成功加载 %d 条历史消息", len(historyMessages))
	} else {
		log.Debugf("未加载到历史消息（可能没有历史记录）")
	}

	return nil
}

// shouldUseRedis 判断是否使用 Redis 作为数据源
func (s *ChatSession) shouldUseRedis() bool {
	// 根据 config_provider.type 判断
	providerType := viper.GetString("config_provider.type")
	return providerType == "redis"
}

// shouldUseManager 判断是否使用 Manager 作为数据源
func (s *ChatSession) shouldUseManager() bool {
	// 根据 config_provider.type 判断
	providerType := viper.GetString("config_provider.type")
	return providerType == "manager"
}

// loadFromManager 从 Manager 数据库加载历史消息
func (s *ChatSession) loadFromManager() ([]*schema.Message, error) {
	// 创建 HistoryClient
	historyCfg := history.HistoryClientConfig{
		BaseURL:   util.GetBackendURL(),
		AuthToken: util.GetManagerAuthToken(),
		Timeout:   viper.GetDuration("manager.history_timeout"),
		Enabled:   true,
	}
	client := history.NewHistoryClient(historyCfg)

	if s.clientState.DeviceID == "" || s.clientState.AgentID == "" {
		return []*schema.Message{}, nil
	}

	req := &history.GetMessagesRequest{
		DeviceID:  s.clientState.DeviceID,
		AgentID:   s.clientState.AgentID,
		SessionID: s.clientState.SessionID,
		Limit:     20,
	}

	resp, err := client.GetMessages(s.ctx, req)
	if err != nil {
		return nil, err
	}

	// 转换为 schema.Message 格式
	messages := make([]*schema.Message, 0, len(resp.Messages))
	for _, item := range resp.Messages {
		var msg *schema.Message
		switch item.Role {
		case "user":
			msg = schema.UserMessage(item.Content)
		case "assistant":
			msg = schema.AssistantMessage(item.Content, item.ToolCalls)
		case "tool":
			msg = schema.ToolMessage(item.Content, item.ToolCallID)
		case "system":
			msg = schema.SystemMessage(item.Content)
		default:
			log.Warnf("未知的消息角色: %s", item.Role)
			continue
		}

		messages = append(messages, msg)
	}

	for _, msg := range messages {
		log.Debugf("历史消息: %+v", msg)
	}

	return messages, nil
}

// 在mqtt 收到type: listen, state: start后进行
func (c *ChatSession) InitAsrLlmTts() error {
	//初始化asr结构
	c.clientState.InitAsr()

	// 初始化memory（memory不在资源池中）
	memoryMode := c.clientState.GetMemoryMode()
	memoryConfig := c.clientState.DeviceConfig.Memory
	memoryType := memory.MemoryType(memoryConfig.Provider)
	if memoryMode != MemoryModeLong {
		memoryType = memory.MemoryTypeNone
	}

	memoryProvider, err := memory.GetProvider(memoryType, memoryConfig.Config)
	if err != nil {
		return fmt.Errorf("创建 Memory 提供者失败: %v", err)
	}
	c.clientState.MemoryProvider = memoryProvider

	if memoryMode == MemoryModeLong {
		// 初始化memory context（仅长记忆模式）
		context, err := memoryProvider.GetContext(c.ctx, c.clientState.GetDeviceIDOrAgentID(), 500)
		if err != nil {
			log.Warnf("初始化memory context失败: %v", err)
		}
		c.clientState.MemoryContext = context
	} else {
		c.clientState.MemoryContext = ""
	}

	return nil
}

// HandleAudioMessage 处理音频消息
func (c *ChatSession) HandleAudioMessage(data []byte) bool {
	select {
	case c.clientState.OpusAudioBuffer <- data:
		return true
	default:
		log.Warnf("音频缓冲区已满, 丢弃音频数据")
	}
	return false
}

// handleListenMessage 处理监听消息
func (s *ChatSession) HandleListenMessage(msg *ClientMessage) error {
	// 根据状态处理
	switch msg.State {
	case MessageStateStart:
		s.HandleListenStart(msg)
	case MessageStateStop:
		s.HandleListenStop()
	case MessageStateDetect:
		s.HandleListenDetect(msg)
	}

	// 记录日志
	log.Infof("设备 %s 更新音频监听状态: %s", msg.DeviceID, msg.State)
	return nil
}

func (s *ChatSession) beginListenStart() uint64 {
	startSeq := s.listenStartSeq.Add(1)
	if s.clientState.IsRealTime() {
		s.realtimeListenSessionActive.Store(true)
	}
	s.clientState.SetListenPhase(ListenPhaseStarting)
	return startSeq
}

func (s *ChatSession) invalidateListenStart() {
	s.listenStartSeq.Add(1)
	s.realtimeListenSessionActive.Store(false)
	s.clientState.SetListenPhase(ListenPhaseIdle)
}

func (s *ChatSession) isCurrentListenStart(startSeq uint64) bool {
	return startSeq == s.listenStartSeq.Load()
}

func (s *ChatSession) isRealtimeListenSessionActive() bool {
	return s.realtimeListenSessionActive.Load()
}

func (s *ChatSession) shouldIgnoreListenStartError(startSeq uint64, ctx context.Context, err error) bool {
	if !s.isCurrentListenStart(startSeq) {
		return true
	}
	if ctx != nil && ctx.Err() != nil {
		return true
	}
	if s.clientState.Ctx.Err() != nil {
		return true
	}
	return errors.Is(err, context.Canceled)
}

func (s *ChatSession) shouldIgnoreAsrLoopError(startSeq uint64, ctx context.Context, err error) bool {
	if !s.isCurrentListenStart(startSeq) {
		return true
	}
	if ctx != nil && ctx.Err() != nil {
		return true
	}
	if s.clientState.Ctx.Err() != nil {
		return true
	}
	return errors.Is(err, context.Canceled)
}

func isWithinCommandTTL(at time.Time, now time.Time) bool {
	return !at.IsZero() && now.Sub(at) <= listenCommandHistoryTTL
}

func isRecentCommand(prev CommandHistorySnapshot, cmdType string, now time.Time) bool {
	return prev.LastCmdType == cmdType && isWithinCommandTTL(prev.LastCmdAt, now)
}

func resolveDetectAction(text string, enableGreeting bool, welcomeAlreadySpoken bool, recentListenStart bool) detectAction {
	if text == "" {
		return detectActionSilent
	}
	if enableGreeting && isWakeupWord(text) {
		if welcomeAlreadySpoken {
			if recentListenStart {
				return detectActionSilent
			}
			return detectActionLLM
		}
		return detectActionWelcome
	}
	return detectActionLLM
}

func (s *ChatSession) cancelPendingDetectLLM() {
	if s == nil {
		return
	}

	s.detectLLMDebounceMu.Lock()
	timer := s.detectLLMDebounceTimer
	s.detectLLMDebounceTimer = nil
	s.detectLLMDebounceMu.Unlock()

	if timer != nil {
		timer.Stop()
	}
}

func (s *ChatSession) scheduleDetectLLM(text string) {
	if s == nil {
		return
	}

	text = strings.TrimSpace(text)
	if text == "" {
		return
	}

	s.cancelPendingDetectLLM()

	var timer *time.Timer
	timer = time.AfterFunc(detectLLMDebounceDuration, func() {
		s.detectLLMDebounceMu.Lock()
		if s.detectLLMDebounceTimer != timer {
			s.detectLLMDebounceMu.Unlock()
			return
		}
		s.detectLLMDebounceTimer = nil
		s.detectLLMDebounceMu.Unlock()

		if s.IsClosing() || s.clientState == nil {
			return
		}
		if s.clientState.Ctx != nil && s.clientState.Ctx.Err() != nil {
			return
		}

		now := time.Now()
		history := s.clientState.GetCommandHistorySnapshot()
		if isRecentCommand(history, CommandTypeListenStart, now) {
			log.Debugf("Detect LLM debounce canceled by recent listen start: history={%s}", history.DebugString(now))
			return
		}
		if phase := s.clientState.GetListenPhase(); phase != ListenPhaseIdle {
			log.Debugf("Detect LLM debounce skipped because listen phase=%s", phase)
			return
		}

		if err := s.AddAsrResultToQueue(text, nil); err != nil {
			log.Errorf("Detect LLM debounce enqueue failed: %v", err)
		}
	})

	s.detectLLMDebounceMu.Lock()
	s.detectLLMDebounceTimer = timer
	s.detectLLMDebounceMu.Unlock()
}

func (s *ChatSession) HandleListenDetect(msg *ClientMessage) error {
	s.cancelPendingDetectLLM()

	// 检查设备激活状态
	if msg.Text != "" {
		isActivated, err := s.CheckDeviceActivated()
		if err != nil {
			log.Errorf("检查设备激活状态失败: %v", err)
			return err
		}
		if !isActivated {
			return nil
		}
	}

	now := time.Now()
	prevHistory := s.clientState.GetCommandHistorySnapshot()
	s.clientState.RecordCommandArrival(CommandTypeDetect, now)

	// 如果有文本，处理
	if msg.Text != "" {
		text := removePunctuation(msg.Text)
		enableGreeting := viper.GetBool("enable_greeting")
		recentListenStart := isRecentCommand(prevHistory, CommandTypeListenStart, now)
		action := resolveDetectAction(text, enableGreeting, s.clientState.IsWelcomeSpeaking, recentListenStart)

		log.Debugf(
			"Detect recv: device=%s text=%q action=%s recentListenStart=%v history={%s} welcomeSpeaking=%v welcomePlaying=%v",
			msg.DeviceID,
			text,
			action,
			recentListenStart,
			prevHistory.DebugString(now),
			s.clientState.IsWelcomeSpeaking,
			s.clientState.IsWelcomePlaying,
		)

		if action == detectActionSilent {
			return nil
		}

		s.StopSpeaking(true)

		if action == detectActionWelcome {
			s.HandleWelcome()
			return nil
		}

		if action == detectActionLLM {
			s.scheduleDetectLLM(text)
		}
	}
	return nil
}

func (s *ChatSession) HandleNotActivated() {
	configProvider, err := user_config.GetProvider(viper.GetString("config_provider.type"))
	if err != nil {
		log.Errorf("获取配置提供者失败: %v", err)
		return
	}

	code, challenge, message, timeoutMs := configProvider.GetActivationInfo(s.clientState.Ctx, s.clientState.DeviceID, "client_id")
	if code == "" {
		log.Errorf("获取激活信息失败: %v", err)
		return
	}

	log.Infof("激活码: %s, 挑战码: %s, 消息: %s, 超时时间: %d", code, challenge, message, timeoutMs)

	s.ttsManager.EnqueueTtsStart(s.clientState.Ctx)
	defer s.ttsManager.EnqueueTtsStop(s.clientState.Ctx)

	sessionCtx := s.clientState.SessionCtx.Get(s.clientState.Ctx)
	ctx := s.clientState.AfterAsrSessionCtx.Get(sessionCtx)
	err = s.ttsManager.handleTextResponse(ctx, llm_common.LLMResponseStruct{
		Text: fmt.Sprintf("请在后台添加设备，激活码: %s", code),
	}, false)
	s.ttsManager.RequestTurnEnd(ctx, err)

}

func (s *ChatSession) HandleWelcome() {
	greetingText := s.GetRandomGreeting()
	sessionCtx := s.clientState.SessionCtx.Get(s.clientState.Ctx)

	// 检查 session 是否已被停止（通过尝试获取锁来判断）
	if !s.stopSpeakingMu.TryLock() {
		log.Debugf("HandleWelcome 正在执行 StopSpeaking，跳过欢迎语")
		return
	}
	s.stopSpeakingMu.Unlock()

	// 检查 sessionCtx 是否已取消
	if sessionCtx.Err() != nil {
		log.Debugf("HandleWelcome sessionCtx 已取消，跳过欢迎语")
		return
	}

	ctx := s.clientState.AfterAsrSessionCtx.Get(sessionCtx)
	// 检查 afterAsrCtx 是否已取消
	if ctx.Err() != nil {
		log.Debugf("HandleWelcome afterAsrCtx 已取消，跳过欢迎语")
		return
	}

	s.clientState.IsWelcomeSpeaking = true
	s.clientState.IsWelcomePlaying = true
	s.ttsManager.EnqueueTtsStart(s.clientState.Ctx)
	err := s.ttsManager.handleTts(ctx, s.ttsManager.currentAudioGeneration(), s.ttsManager.currentTtsMetricCycle(), llm_common.LLMResponseStruct{Text: greetingText}, nil, nil)
	s.ttsManager.RequestTurnEnd(ctx, err)
	s.ttsManager.EnqueueTtsStop(s.clientState.Ctx)
}

func (a *ChatSession) checkExitWords(text string) bool {
	exitWords := []string{"再见", "退下吧", "退出", "退出对话"}
	for _, word := range exitWords {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

func normalizeOpenClawKeywordText(text string) string {
	return removePunctuation(strings.ToLower(strings.TrimSpace(text)))
}

func containsOpenClawKeyword(text string, keywords []string) bool {
	normalizedText := normalizeOpenClawKeywordText(text)
	if normalizedText == "" {
		return false
	}
	for _, keyword := range keywords {
		normalizedKeyword := normalizeOpenClawKeywordText(keyword)
		if normalizedKeyword == "" {
			continue
		}
		if strings.Contains(normalizedText, normalizedKeyword) {
			return true
		}
	}
	return false
}

func (s *ChatSession) isOpenClawEnterKeyword(text string) bool {
	return containsOpenClawKeyword(text, s.clientState.DeviceConfig.OpenClaw.EnterKeywords)
}

func (s *ChatSession) isOpenClawExitKeyword(text string) bool {
	return containsOpenClawKeyword(text, s.clientState.DeviceConfig.OpenClaw.ExitKeywords)
}

func openClawLogSnippet(text string, maxRunes int) string {
	if maxRunes <= 0 {
		return ""
	}
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return ""
	}
	runes := []rune(trimmed)
	if len(runes) <= maxRunes {
		return string(runes)
	}
	return string(runes[:maxRunes]) + "..."
}

func (s *ChatSession) GetRandomGreeting() string {
	greetingList := viper.GetStringSlice("greeting_list")
	if len(greetingList) == 0 {
		return "你好，有啥好玩的."
	}
	rand.Seed(time.Now().UnixNano())
	return greetingList[rand.Intn(len(greetingList))]
}

func (s *ChatSession) AddTextToTTSQueue(text string) error {
	return s.llmManager.AddTextToTTSQueue(text)
}

func (s *ChatSession) AddTextToTTSQueueWithOptions(text string, options llmResponseChannelOptions) error {
	return s.llmManager.AddTextToTTSQueueWithOptions(text, options)
}

func (s *ChatSession) IsTTSActive() bool {
	if s == nil || s.ttsManager == nil {
		return false
	}
	return s.ttsManager.ttsActive.Load()
}

func (s *ChatSession) getOrCreateOpenClawStream(correlationID string) (chan llm_common.LLMResponseStruct, bool, error) {
	correlationID = strings.TrimSpace(correlationID)
	if correlationID == "" {
		return nil, false, fmt.Errorf("missing correlation_id")
	}

	s.openClawStreamMu.Lock()
	if existing, ok := s.openClawStreams[correlationID]; ok {
		s.openClawStreamMu.Unlock()
		return existing, false, nil
	}
	streamChan := make(chan llm_common.LLMResponseStruct, 16)
	s.openClawStreams[correlationID] = streamChan
	s.openClawStreamMu.Unlock()

	sessionCtx := s.clientState.SessionCtx.Get(s.clientState.Ctx)
	ctx := s.clientState.AfterAsrSessionCtx.Get(sessionCtx)
	options := llmResponseChannelOptions{}
	hasWarmup := s.getOpenClawWarmupTask(correlationID) != nil
	if hasWarmup {
		options.disableTTSCommands = true
		options.onEndFunc = func(err error, args ...any) {
			// 暖场接管了 start，正式 OpenClaw 回复收尾时需要在这里补回 stop；
			// 不能放在暖场切换点发送，否则会把主回复中途截断。
			if !s.clientState.IsRealTime() {
				s.ttsManager.EnqueueTtsStop(ctx)
			}
			s.ttsManager.RequestTurnEnd(ctx, err)
			s.finishOpenClawWarmup(correlationID, false)
		}
	}
	log.Infof("OpenClaw stream created: device=%s correlation_id=%s warmup_attached=%v", s.clientState.DeviceID, correlationID, hasWarmup)
	if err := s.llmManager.HandleLLMResponseChannelAsyncWithOptions(ctx, nil, streamChan, options); err != nil {
		if hasWarmup && !s.clientState.IsRealTime() {
			s.ttsManager.EnqueueTtsStop(ctx)
		}
		if hasWarmup {
			s.ttsManager.RequestTurnEnd(ctx, err)
		}
		s.openClawStreamMu.Lock()
		delete(s.openClawStreams, correlationID)
		s.openClawStreamMu.Unlock()
		close(streamChan)
		return nil, false, err
	}

	return streamChan, true, nil
}

func (s *ChatSession) closeOpenClawStream(correlationID string) {
	correlationID = strings.TrimSpace(correlationID)
	if correlationID == "" {
		return
	}
	s.openClawStreamMu.Lock()
	delete(s.openClawStreams, correlationID)
	s.openClawStreamMu.Unlock()
}

func (s *ChatSession) clearOpenClawStreams() {
	s.openClawStreamMu.Lock()
	s.openClawStreams = make(map[string]chan llm_common.LLMResponseStruct)
	s.openClawStreamMu.Unlock()
}

func (s *ChatSession) clearPendingSpeakerResult() {
	if s == nil {
		return
	}

	s.speakerResultMu.Lock()
	s.pendingSpeakerResult = nil
	s.speakerResultMu.Unlock()

	for {
		select {
		case <-s.speakerResultReady:
		default:
			return
		}
	}
}

func (s *ChatSession) InjectOpenClawResponse(event openclaw.ResponseDelivery) error {
	correlationID := strings.TrimSpace(event.CorrelationID)
	text := strings.TrimSpace(event.Text)

	// 非流式兜底：没有 correlation_id 时直接按单句注入。
	if correlationID == "" {
		if text == "" {
			return nil
		}
		return s.AddTextToTTSQueue(text)
	}

	// 中间空分片没有意义，直接跳过；结束空分片保留用于收尾。
	if text == "" && !event.IsEnd {
		return nil
	}

	streamChan, created, err := s.getOrCreateOpenClawStream(correlationID)
	if err != nil {
		return err
	}

	isStart := event.IsStart
	if created && !isStart {
		// 若首个到达分片没有标 start，兜底拉起首段。
		isStart = true
	}
	if isStart {
		if task := s.getOpenClawWarmupTask(correlationID); task != nil {
			if text != "" {
				// 仅在第一段真正可播正文到达时才停掉暖场，避免被过短前导分片过早抢占。
				// 暖场自己的首段标记只用于暖场 TTS，不能吞掉正式回复首段的 IsStart，
				// 否则正式回复会降级成单句 TTS，后续 snapshot 也会被当成第二句再次播报。
				s.cancelOpenClawWarmup(correlationID, false)
				s.beginOpenClawSpeech(task)
			} else {
				isStart = false
			}
		}
	} else if event.IsEnd {
		s.cancelOpenClawWarmup(correlationID, false)
	}

	resp := llm_common.LLMResponseStruct{
		Text:    text,
		IsStart: isStart,
		IsEnd:   event.IsEnd,
	}

	select {
	case <-s.ctx.Done():
		return fmt.Errorf("chat session closed")
	case streamChan <- resp:
	}

	if event.IsEnd {
		s.closeOpenClawStream(correlationID)
	}

	return nil
}

// InterruptAndClearTTSQueue 触发 TTS 打断并清空发送队列（供 realtime 模式 VAD 打断等场景调用）
func (s *ChatSession) InterruptAndClearTTSQueue() {
	if s.mediaPlayer != nil {
		if err := s.mediaPlayer.Suspend(); err != nil && !errors.Is(err, context.Canceled) {
			log.Warnf("挂起媒体播放失败: %v", err)
		}
	}
	s.ttsManager.InterruptAndStop(s.clientState.Ctx, true, context.Canceled)
}

// handleAbortMessage 处理中止消息
func (s *ChatSession) HandleAbortMessage(msg *ClientMessage) error {
	s.cancelPendingDetectLLM()

	// 设置打断状态
	s.clientState.Abort = true

	if s.clientState.IsRealTime() {
		s.StopSpeakingAfterAsr(true)
	} else {
		s.StopSpeaking(true)
	}

	// 记录日志
	log.Infof("设备 %s abort 会话", msg.DeviceID)
	return nil
}

func (s *ChatSession) CheckDeviceActivated() (bool, error) {
	if viper.GetBool("auth.enable") {
		if !s.clientState.IsActivated {
			const falseCheckThrottle = time.Second
			s.activationCheckMu.Lock()
			lastFalseAt := s.lastActivationFalseAt
			s.activationCheckMu.Unlock()
			if !lastFalseAt.IsZero() && time.Since(lastFalseAt) < falseCheckThrottle {
				log.Debugf("设备 %s 激活状态仍为未激活，跳过重复实时校验", s.clientState.DeviceID)
				return false, nil
			}

			configProvider, err := user_config.GetProvider(viper.GetString("config_provider.type"))
			if err != nil {
				log.Errorf("获取配置提供者失败: %v", err)
				return false, err
			}
			//调用接口再次确认激活状态
			isActivated, err := configProvider.IsDeviceActivated(s.clientState.Ctx, s.clientState.DeviceID, "client_id")
			if err != nil {
				log.Errorf("获取激活状态失败: %v", err)
				return false, err
			}
			if isActivated {
				s.clientState.IsActivated = true
				s.activationCheckMu.Lock()
				s.lastActivationFalseAt = time.Time{}
				s.activationCheckMu.Unlock()
			} else {
				s.activationCheckMu.Lock()
				s.lastActivationFalseAt = time.Now()
				s.activationCheckMu.Unlock()
				s.HandleNotActivated()
				return false, nil
			}
		}
	}
	return true, nil
}

func (s *ChatSession) HandleListenStart(msg *ClientMessage) error {
	s.cancelPendingDetectLLM()

	// 先检查激活状态
	isActivated, err := s.CheckDeviceActivated()
	if err != nil {
		log.Errorf("检查设备激活状态失败: %v", err)
		return err
	}
	if !isActivated {
		return nil
	}

	now := time.Now()
	prevHistory := s.clientState.GetCommandHistorySnapshot()
	recentDetect := isRecentCommand(prevHistory, CommandTypeDetect, now)

	if recentDetect && s.clientState.IsWelcomePlaying {
		log.Infof("设备 %s 欢迎语播放中，忽略 listen start: history={%s}", msg.DeviceID, prevHistory.DebugString(now))
		return nil
	}

	log.Debugf(
		"ListenStart recv: device=%s mode=%s recentDetect=%v history={%s} welcomeSpeaking=%v welcomePlaying=%v phase=%s",
		msg.DeviceID,
		msg.Mode,
		recentDetect,
		prevHistory.DebugString(now),
		s.clientState.IsWelcomeSpeaking,
		s.clientState.IsWelcomePlaying,
		s.clientState.GetListenPhase(),
	)

	// realtime 模式首次启动：跳过欢迎语判断和 Destroy，直接进入监听
	if msg.Mode == "realtime" {
		// 当前 realtime listen 会话尚未走到 listen stop / session cancel / close 时，
		// 重复 listen start 包统一静默忽略，避免打断当前链路。
		if s.clientState.IsRealTime() && s.isRealtimeListenSessionActive() {
			return nil
		}

		s.clientState.RecordCommandArrival(CommandTypeListenStart, now)
		s.StopSpeaking(true)

		s.clientState.ListenMode = msg.Mode
		log.Infof("设备 %s 拾音模式: %s", msg.DeviceID, msg.Mode)

		shouldStartAudioIdleWindow := s.clientState.GetListenPhase() != ListenPhaseListening
		startSeq := s.beginListenStart()
		go func() {
			if err := s.OnListenStart(startSeq, shouldStartAudioIdleWindow); err != nil {
				log.Errorf("设备 %s listen start 启动失败: %v", msg.DeviceID, err)
			}
		}()
		return nil
	}

	if s.clientState.GetListenPhase() == ListenPhaseStarting {
		log.Infof("设备 %s listen start 正在启动中，忽略重复 listen start", msg.DeviceID)
		return nil
	}

	s.clientState.RecordCommandArrival(CommandTypeListenStart, now)

	// 处理拾音模式
	s.clientState.ListenMode = msg.Mode
	log.Infof("设备 %s 拾音模式: %s", msg.DeviceID, msg.Mode)
	//if s.clientState.ListenMode == "manual" {
	s.StopSpeaking(true)
	//}

	startSeq := s.beginListenStart()
	go func() {
		if err := s.OnListenStart(startSeq, true); err != nil {
			log.Errorf("设备 %s listen start 启动失败: %v", msg.DeviceID, err)
		}
	}()

	return nil
}

func (s *ChatSession) HandleListenStop() error {
	s.cancelPendingDetectLLM()
	s.clientState.RecordCommandArrival(CommandTypeListenStop, time.Now())
	/*if s.clientState.ListenMode == "auto" {
		s.clientState.CancelSessionCtx()
	}*/

	//调用
	if s.clientState.IsRealTime() {
		s.invalidateListenStart()
	}
	s.clientState.OnManualStop()

	return nil
}

func (s *ChatSession) OnListenStart(startSeq uint64, shouldStartAudioIdleWindow bool) error {
	log.Debugf("OnListenStart start")
	defer log.Debugf("OnListenStart end")

	if !s.isCurrentListenStart(startSeq) {
		log.Debugf("OnListenStart stale before init, skip")
		return nil
	}

	select {
	case <-s.clientState.Ctx.Done():
		log.Debugf("OnListenStart Ctx done, return")
		if s.isCurrentListenStart(startSeq) {
			s.clientState.SetListenPhase(ListenPhaseIdle)
		}
		return nil
	default:
	}

	// realtime 模式：跳过 Destroy，保持 ASR 持续运行，但清空 AudioBuffer
	if s.clientState.IsRealTime() {
		s.clientState.AsrAudioBuffer.ClearAsrAudioData()
	} else {
		s.clientState.Destroy()
		if !s.isCurrentListenStart(startSeq) {
			log.Debugf("OnListenStart stale after destroy, skip")
			return nil
		}
	}

	s.clientState.SetListenPhase(ListenPhaseStarting)

	s.clientState.SetStatus(ClientStatusListening)

	ctx := s.clientState.SessionCtx.Get(s.clientState.Ctx)

	//初始化asr相关
	if s.clientState.ListenMode == "manual" {
		s.clientState.VoiceStatus.SetClientHaveVoice(true)
	}

	// 启动asr流式识别，复用 restartAsrRecognition 函数
	if !s.isCurrentListenStart(startSeq) {
		log.Debugf("OnListenStart stale before ASR restart, skip")
		return nil
	}
	err := s.asrManager.RestartAsrRecognition(ctx)
	if err != nil {
		if s.shouldIgnoreListenStartError(startSeq, ctx, err) {
			log.Infof("OnListenStart interrupted during ASR restart, ignore err: %v", err)
			if s.isCurrentListenStart(startSeq) {
				s.clientState.SetListenPhase(ListenPhaseIdle)
			}
			return nil
		}

		log.Errorf("asr流式识别失败: %v", err)
		if s.isCurrentListenStart(startSeq) {
			s.clientState.SetListenPhase(ListenPhaseIdle)
		}
		s.CloseWithReason(chatSessionCloseReasonFatalError)
		return err
	}

	if !s.isCurrentListenStart(startSeq) {
		log.Debugf("OnListenStart stale after ASR restart, cancel current start")
		s.clientState.Asr.CancelWithReason("ChatSession.OnListenStart: stale listen start after ASR restart")
		return nil
	}

	s.clientState.SetListenPhase(ListenPhaseListening)
	if shouldStartAudioIdleWindow {
		s.clientState.StartAudioIdleWindow(time.Now())
	}

	// 定义消息保存回调
	onMessageSave := func(userMsg *schema.Message, messageID string, audioData []float32) {
		// ASR 文本和音频同时获取，一次性保存（不需要两阶段）
		eventbus.Get().Publish(eventbus.TopicAddMessage, &eventbus.AddMessageEvent{
			ClientState: s.clientState,
			Msg:         *userMsg,
			MessageID:   messageID,
			AudioData:   [][]byte{util.Float32SliceToBytes(audioData)}, // 转换为字节数组
			AudioSize:   len(audioData) * 4,                            // float32 = 4 bytes
			SampleRate:  s.clientState.InputAudioFormat.SampleRate,
			Channels:    s.clientState.InputAudioFormat.Channels,
			IsUpdate:    false, // 一次性保存（文本+音频）
			Timestamp:   time.Now(),
		})
	}

	// 定义错误处理回调
	onError := func(err error) {
		if s.shouldIgnoreAsrLoopError(startSeq, ctx, err) {
			log.Infof("ASR识别循环在重置/退出中结束，忽略 err: %v", err)
			return
		}
		log.Errorf("ASR识别循环错误: %v", err)
		s.CloseWithReason(chatSessionCloseReasonFatalError)
	}

	// 启动ASR识别结果处理循环（资源管理在 ASRManager 内部）
	s.asrManager.StartAsrRecognitionLoop(ctx, onMessageSave, onError)

	return nil
}

// startChat 开始对话
func (s *ChatSession) AddAsrResultToQueue(text string, speakerResult *speaker.IdentifyResult) error {
	return s.AddAsrResultToQueueWithOptions(text, speakerResult, llmResponseChannelOptions{})
}

func (s *ChatSession) AddAsrResultToQueueWithOptions(text string, speakerResult *speaker.IdentifyResult, options llmResponseChannelOptions) error {
	log.Debugf("AddAsrResultToQueue text: %s", text)
	if speakerResult != nil && speakerResult.Identified {
		log.Debugf("AddAsrResultToQueue speaker: %s (confidence: %.2f)", speakerResult.SpeakerName, speakerResult.Confidence)
	}

	// 检查 session 是否已被停止（通过尝试获取锁来判断）
	// 如果 StopSpeaking 正在执行，这里会等待；如果已执行完成，tryLock 会立即返回
	if !s.stopSpeakingMu.TryLock() {
		log.Debugf("AddAsrResultToQueue 正在执行 StopSpeaking，丢弃消息")
		return nil
	}
	s.stopSpeakingMu.Unlock()

	sessionCtx := s.clientState.SessionCtx.Get(s.clientState.Ctx)
	// 检查 sessionCtx 是否已取消
	if sessionCtx.Err() != nil {
		log.Debugf("AddAsrResultToQueue sessionCtx 已取消，丢弃消息")
		return nil
	}
	ctx := s.clientState.AfterAsrSessionCtx.Get(sessionCtx)
	ctx = withTTSPlaybackStartHook(ctx, options.onTTSPlaybackStart)
	ctx = withTTSTurnEndPolicy(ctx, options.ttsTurnEndPolicy)

	item := AsrResponseChannelItem{
		ctx:           ctx,
		text:          text,
		speakerResult: speakerResult,
	}
	err := s.chatTextQueue.Push(item)
	if err != nil {
		log.Warnf("chatTextQueue 已满或已关闭, 丢弃消息")
	}
	return nil
}

func (s *ChatSession) processChatText(ctx context.Context) {
	log.Debugf("processChatText start")
	defer log.Debugf("processChatText end")

	for {
		item, err := s.chatTextQueue.Pop(ctx, 0)
		if err != nil {
			if err == util.ErrQueueCtxDone {
				return
			}
			continue
		}

		err = s.actionDoChat(item.ctx, item.text, item.speakerResult)
		if err != nil {
			log.Errorf("处理对话失败: %v", err)
			continue
		}
	}
}

func (s *ChatSession) ClearChatTextQueue() {
	s.chatTextQueue.Clear()
}

// DoExitChat 执行退出聊天逻辑（发送再见语并关闭会话）
func (s *ChatSession) DoExitChat() {
	// 友好的再见语
	goodbyeText := "好的，再见！期待下次与您聊天～"

	// 保存一条 assistant 角色的消息
	goodbyeMsg := schema.AssistantMessage(goodbyeText, nil)
	if err := s.llmManager.AddLlmMessage(s.clientState.Ctx, goodbyeMsg); err != nil {
		log.Errorf("保存再见消息失败: %v", err)
	}

	// 获取 context
	sessionCtx := s.clientState.SessionCtx.Get(s.clientState.Ctx)
	ctx := s.clientState.AfterAsrSessionCtx.Get(sessionCtx)

	// 发送 TTS 再见语
	s.ttsManager.EnqueueTtsStart(ctx)

	err := s.ttsManager.handleTextResponse(ctx, llm_common.LLMResponseStruct{
		Text:    goodbyeText,
		IsStart: true,
		IsEnd:   true,
	}, true) // 同步处理，等待TTS完成

	if err != nil {
		log.Errorf("发送再见语失败: %v", err)
	}

	s.ttsManager.RequestTurnEnd(ctx, err)
	s.ttsManager.EnqueueTtsStop(ctx)
	// 关闭会话
	s.CloseWithReason(chatSessionCloseReasonExplicitExit)
}

func (s *ChatSession) Close() {
	s.CloseWithReason(chatSessionCloseReasonManagerShutdown)
}

func (s *ChatSession) IsClosing() bool {
	if s == nil {
		return true
	}
	return s.closing.Load()
}

func (s *ChatSession) CloseWithReason(reason string) {
	s.closing.Store(true)
	s.closeOnce.Do(func() {
		// 清理ASR资源（资源管理在 ASRManager 内部）
		if s.asrManager != nil {
			s.asrManager.Cleanup()
		}
		deviceID := ""
		if s.clientState != nil {
			deviceID = s.clientState.DeviceID
		}
		log.Debugf("ChatSession.Close() 开始清理会话资源, 设备 %s", deviceID)

		if s.mediaPlayer != nil {
			s.mediaPlayer.DetachSession(true)
		}

		s.cancelPendingDetectLLM()

		// 取消会话级别的上下文
		if s.cancel != nil {
			s.cancel()
		}
		s.finishOpenClawWarmup("", false)

		// 清理聊天文本队列
		s.ClearChatTextQueue()
		s.clearOpenClawStreams()

		// 停止说话和清理音频相关资源。Close 路径前面已经 DetachSession(true)，
		// 这里不要再次 Suspend 媒体，否则会把 resumeOnAttach 清掉。
		s.stopSpeakingWithLock(true, true, false)

		if s.speakerManager != nil {
			s.speakerManager.Close()
		}

		if s.clientState != nil {
			eventbus.Get().Publish(eventbus.TopicSessionEnd, s.clientState)
		}

		log.Debugf("ChatSession.Close() 会话资源清理完成, 设备 %s", deviceID)

		if s.closeHandler != nil {
			s.closeHandler(s, reason)
		}
	})
}

func (s *ChatSession) actionDoChat(ctx context.Context, text string, speakerResult *speaker.IdentifyResult) error {
	select {
	case <-ctx.Done():
		log.Debugf("actionDoChat ctx done, return")
		return nil
	default:
	}

	agentID := strings.TrimSpace(s.clientState.AgentID)
	deviceID := strings.TrimSpace(s.clientState.DeviceID)
	openclawSessionID := strings.TrimSpace(s.clientState.SessionID)
	trimmedText := strings.TrimSpace(text)

	handledByRealtimeGate, gateErr := s.tryHandleRealtimeMcpAudioASR(ctx, trimmedText)
	if handledByRealtimeGate {
		return gateErr
	}

	openclawManager := openclaw.GetManager()
	if s.clientState.DeviceConfig.OpenClaw.Allowed {
		isOpenClawMode := openclawManager.IsModeEnabled(agentID, deviceID)
		isEnterKeyword := s.isOpenClawEnterKeyword(text)
		isExitKeyword := false
		if isOpenClawMode {
			isExitKeyword = s.isOpenClawExitKeyword(text)
		}
		log.Debugf(
			"OpenClaw路由判定: agent=%s device=%s session=%s allowed=%v mode=%v enter_keyword=%v exit_keyword=%v text_len=%d text_trim_len=%d text_snippet=%q",
			agentID,
			deviceID,
			openclawSessionID,
			s.clientState.DeviceConfig.OpenClaw.Allowed,
			isOpenClawMode,
			isEnterKeyword,
			isExitKeyword,
			len(text),
			len(trimmedText),
			openClawLogSnippet(trimmedText, 64),
		)
		if isOpenClawMode {
			if isExitKeyword {
				s.finishOpenClawWarmup("", true)
				exited := openclawManager.ExitMode(agentID, deviceID)
				_ = s.AddTextToTTSQueue("已退出OpenClaw模式")
				log.Infof("设备 %s 退出OpenClaw模式: agent=%s exited=%v", deviceID, agentID, exited)
				return nil
			}

			log.Infof(
				"OpenClaw发送STT: agent=%s device=%s session=%s text_len=%d text_snippet=%q",
				agentID,
				deviceID,
				openclawSessionID,
				len(trimmedText),
				openClawLogSnippet(trimmedText, 64),
			)
			s.finishOpenClawWarmup("", true)
			messageID, err := openclawManager.SendMessage(
				agentID,
				deviceID,
				text,
				openclawSessionID,
			)
			if err != nil {
				log.Warnf(
					"设备 %s OpenClaw消息发送失败，已回退普通模式: agent=%s session=%s text_snippet=%q err=%v",
					deviceID,
					agentID,
					openclawSessionID,
					openClawLogSnippet(trimmedText, 64),
					err,
				)
				openclawManager.ExitMode(agentID, deviceID)
				_ = s.AddTextToTTSQueue("OpenClaw当前不可用，已退出OpenClaw模式")
			} else {
				s.startOpenClawWarmup(messageID, text)
				log.Infof("OpenClaw发送STT成功: agent=%s device=%s session=%s message_id=%s", agentID, deviceID, openclawSessionID, messageID)
			}
			return nil
		}

		if isEnterKeyword {
			if !openclawManager.EnterMode(agentID, deviceID) {
				_ = s.AddTextToTTSQueue("OpenClaw当前不可用，请稍后再试")
				log.Warnf("设备 %s 进入OpenClaw模式失败: agent=%s agent session not ready", deviceID, agentID)
				return nil
			}
			_ = s.AddTextToTTSQueue("已进入OpenClaw模式，请继续说")
			log.Infof("设备 %s 进入OpenClaw模式: agent=%s trigger=%q", deviceID, agentID, openClawLogSnippet(trimmedText, 32))
			return nil
		}
		log.Debugf(
			"OpenClaw未接管当前STT: agent=%s device=%s mode=%v enter_keyword=%v text_snippet=%q",
			agentID,
			deviceID,
			isOpenClawMode,
			isEnterKeyword,
			openClawLogSnippet(trimmedText, 64),
		)
	} else {
		s.finishOpenClawWarmup("", false)
		if openclawManager.ExitMode(agentID, deviceID) {
			log.Debugf("OpenClaw配置未开启，已强制退出模式: agent=%s device=%s", agentID, deviceID)
		}
	}

	if s.checkExitWords(text) {
		// 发布退出聊天事件
		eventbus.Get().Publish(eventbus.TopicExitChat, &eventbus.ExitChatEvent{
			ClientState: s.clientState,
			Reason:      "用户主动退出",
			TriggerType: "exit_words",
			UserText:    text,
			Timestamp:   time.Now(),
		})
		return nil
	}

	clientState := s.clientState

	sessionID := clientState.SessionID

	// 声纹识别后动态切换TTS（未识别到时恢复默认TTS）
	if err := s.switchTTSForSpeaker(speakerResult); err != nil {
		log.Warnf("切换TTS失败: %v", err)
		// 不中断流程，继续使用当前TTS
	}

	// 直接创建Eino原生消息
	userMessage := &schema.Message{
		Role:    schema.User,
		Content: text,
	}

	// 获取全局MCP工具列表
	mcpTools, err := mcp.GetToolsByDeviceIdWithTransport(
		clientState.DeviceID,
		clientState.AgentID,
		s.serverTransport.GetTransportType(),
		clientState.DeviceConfig.MCPServiceNames,
	)
	if err != nil {
		log.Errorf("获取设备 %s 的工具失败: %v", clientState.DeviceID, err)
		mcpTools = make(map[string]tool.InvokableTool)
	}
	if !hasAvailableKnowledgeBase(clientState.DeviceConfig.KnowledgeBases) {
		if _, ok := mcpTools["search_knowledge"]; ok {
			delete(mcpTools, "search_knowledge")
			log.Infof("设备 %s 未关联可用知识库，已移除工具 search_knowledge", clientState.DeviceID)
		}
	}

	// 将MCP工具转换为接口格式以便传递给转换函数
	mcpToolsInterface := make(map[string]interface{})
	for name, tool := range mcpTools {
		mcpToolsInterface[name] = tool
	}

	// 转换MCP工具为Eino ToolInfo格式
	einoTools, err := llm.ConvertMCPToolsToEinoTools(ctx, mcpToolsInterface)
	if err != nil {
		log.Errorf("转换MCP工具失败: %v", err)
		einoTools = nil
	}

	toolNameList := make([]string, 0)
	for _, tool := range einoTools {
		toolNameList = append(toolNameList, tool.Name)
	}

	// 发送带工具的LLM请求
	log.Infof("使用 %d 个MCP工具发送LLM请求, tools: %+v", len(einoTools), toolNameList)

	err = s.llmManager.DoLLmRequest(ctx, userMessage, einoTools, true, speakerResult)
	if err != nil {
		log.Errorf("发送带工具的 LLM 请求失败, seesionID: %s, error: %v", sessionID, err)
		return fmt.Errorf("发送带工具的 LLM 请求失败: %v", err)
	}
	return nil
}

func hasAvailableKnowledgeBase(knowledgeBases []types.KnowledgeBaseRef) bool {
	for _, kb := range knowledgeBases {
		if strings.EqualFold(strings.TrimSpace(kb.Status), "inactive") {
			continue
		}
		if strings.TrimSpace(kb.ExternalKBID) == "" {
			continue
		}
		return true
	}
	return false
}

func (s *ChatSession) MarkTurnSpeakerInterrupted() {
	if s == nil {
		return
	}
	s.turnSpeakerInterrupted.Store(true)
}

func (s *ChatSession) ConsumeTurnSpeakerInterrupted() bool {
	if s == nil {
		return false
	}
	return s.turnSpeakerInterrupted.Swap(false)
}

func (s *ChatSession) ResetTurnSpeakerInterrupted() {
	if s == nil {
		return
	}
	s.turnSpeakerInterrupted.Store(false)
}

func (s *ChatSession) ShouldAllowSpeakerChat(speakerResult *speaker.IdentifyResult, speakerInterrupted bool) (bool, string) {
	if s == nil || s.clientState == nil {
		return true, ""
	}

	matchedConfiguredSpeaker := s.clientState.HasMatchedConfiguredSpeaker(speakerResult)
	if speakerInterrupted && !matchedConfiguredSpeaker {
		return false, "speaker_interrupt_without_identify"
	}

	if s.clientState.RequireMatchedSpeakerForChat() && !matchedConfiguredSpeaker {
		return false, "speaker_chat_mode_identified_only_not_matched"
	}

	return true, ""
}

// switchTTSForSpeaker 为识别的说话人切换TTS
func (s *ChatSession) switchTTSForSpeaker(speakerResult *speaker.IdentifyResult) error {
	s.clientState.SpeakerTTSConfig = nil

	// 1. 检查 speakerResult 是否为 nil
	if speakerResult == nil {
		log.Debug("speakerResult 为 nil，清空声纹TTS配置")
		return nil
	}

	// 2. 查找声纹组配置
	speakerGroupInfo, found := s.clientState.DeviceConfig.VoiceIdentify[speakerResult.SpeakerName]
	if !found {
		// 未找到配置，清空声纹TTS配置
		log.Debugf("未找到声纹组 %s 的配置，清空声纹TTS配置", speakerResult.SpeakerName)
		return nil
	}

	// 3. 检查是否配置了自定义音色
	if speakerGroupInfo.TTSConfigID == nil || *speakerGroupInfo.TTSConfigID == "" {
		// 未配置自定义音色，清空声纹TTS配置
		log.Debugf("声纹组 %s 未配置自定义TTS，清空声纹TTS配置", speakerResult.SpeakerName)
		return nil
	}

	// 4. 从系统配置（viper）中查找对应的TTS配置
	var targetTTSConfig *types.TtsConfigItem
	ttsConfigsRaw := viper.Get("tts")
	if ttsConfigsRaw == nil {
		return fmt.Errorf("系统配置中未找到 tts")
	}

	// 解析 tts 配置（现在是一个 map，key 是 config_id）
	if ttsConfigsMap, ok := ttsConfigsRaw.(map[string]interface{}); ok {
		// 查找匹配的 config_id
		if configItem, exists := ttsConfigsMap[*speakerGroupInfo.TTSConfigID]; exists {
			if configMap, ok := configItem.(map[string]interface{}); ok {
				// 解析配置项
				ttsItem := &types.TtsConfigItem{
					ConfigID: *speakerGroupInfo.TTSConfigID,
				}
				if name, ok := configMap["name"].(string); ok {
					ttsItem.Name = name
				}
				if provider, ok := configMap["provider"].(string); ok {
					ttsItem.Provider = provider
				}
				if isDefault, ok := configMap["is_default"].(bool); ok {
					ttsItem.IsDefault = isDefault
				}
				// 配置项的其他字段直接作为 config
				ttsItem.Config = make(map[string]interface{})
				for k, v := range configMap {
					if k != "name" && k != "provider" && k != "is_default" && k != "config_id" {
						ttsItem.Config[k] = v
					}
				}
				targetTTSConfig = ttsItem
			}
		}
	}

	if targetTTSConfig == nil {
		return fmt.Errorf("未找到TTS配置 %s", *speakerGroupInfo.TTSConfigID)
	}

	// 5. 复制TTS配置以避免修改原始配置
	ttsConfig := make(map[string]interface{})
	for k, v := range targetTTSConfig.Config {
		ttsConfig[k] = v
	}

	// 6. 如果配置了音色值，覆盖到TTS配置中
	if speakerGroupInfo.Voice != nil && *speakerGroupInfo.Voice != "" {
		// 根据provider设置对应的音色字段
		if targetTTSConfig.Provider == "cosyvoice" {
			ttsConfig["spk_id"] = *speakerGroupInfo.Voice
		} else {
			ttsConfig["voice"] = *speakerGroupInfo.Voice
		}
		log.Debugf("为说话人 %s 设置音色: %s", speakerResult.SpeakerName, *speakerGroupInfo.Voice)
	}
	if targetTTSConfig.Provider == "aliyun_qwen" &&
		speakerGroupInfo.VoiceModelOverride != nil &&
		strings.TrimSpace(*speakerGroupInfo.VoiceModelOverride) != "" {
		overrideModel := strings.TrimSpace(*speakerGroupInfo.VoiceModelOverride)
		ttsConfig["model"] = overrideModel
		log.Debugf("为说话人 %s 覆盖千问模型: %s", speakerResult.SpeakerName, overrideModel)
	}

	// 7. 保存完整的 TTS 配置（深拷贝）
	s.clientState.SpeakerTTSConfig = make(map[string]interface{})
	for k, v := range ttsConfig {
		s.clientState.SpeakerTTSConfig[k] = v
	}
	// 确保 provider 在 config 中
	s.clientState.SpeakerTTSConfig["provider"] = targetTTSConfig.Provider

	log.Infof("✅ 为说话人 %s 切换TTS配置成功 - Provider: %s, ConfigID: %s, Voice: %v",
		speakerResult.SpeakerName,
		targetTTSConfig.Provider,
		targetTTSConfig.ConfigID,
		speakerGroupInfo.Voice)

	return nil
}

func (s *ChatSession) hookContext(ctx context.Context) chathooks.Context {
	sessionID := ""
	deviceID := ""
	if s != nil && s.clientState != nil {
		sessionID = s.clientState.SessionID
		deviceID = s.clientState.DeviceID
	}

	return chathooks.Context{
		Ctx:       ctx,
		SessionID: sessionID,
		DeviceID:  deviceID,
	}
}

func (s *ChatSession) emitMetricStage(ctx context.Context, stage chathooks.MetricStage, ts int64, err error) {
	if s == nil {
		return
	}

	hookErr := s.hookHub.EmitMetric(s.hookContext(ctx), chathooks.MetricData{Stage: stage, Ts: ts, Err: err})
	if hookErr != nil {
		log.Warnf("METRIC hook 执行失败: stage=%s err=%v", stage, hookErr)
	}
}

func (s *ChatSession) TraceTurnStart(ctx context.Context, ts int64) {
	s.emitMetricStage(ctx, chathooks.MetricTurnStart, ts, nil)
}

func (s *ChatSession) TraceTurnEnd(ctx context.Context, ts int64, err error) {
	s.emitMetricStage(ctx, chathooks.MetricTurnEnd, ts, err)
}

func (s *ChatSession) TraceAsrFirstText(ctx context.Context, ts int64) {
	s.emitMetricStage(ctx, chathooks.MetricAsrFirstText, ts, nil)
}

func (s *ChatSession) TraceAsrFinalText(ctx context.Context, ts int64) {
	s.emitMetricStage(ctx, chathooks.MetricAsrFinalText, ts, nil)
}

func (s *ChatSession) TraceLlmStart(ctx context.Context, ts int64) {
	s.emitMetricStage(ctx, chathooks.MetricLlmStart, ts, nil)
}

func (s *ChatSession) TraceLlmFirstToken(ctx context.Context, ts int64) {
	s.emitMetricStage(ctx, chathooks.MetricLlmFirstToken, ts, nil)
}

func (s *ChatSession) TraceLlmEnd(ctx context.Context, ts int64, err error) {
	s.emitMetricStage(ctx, chathooks.MetricLlmEnd, ts, err)
}

func (s *ChatSession) TraceTtsStart(ctx context.Context, ts int64) {
	s.emitMetricStage(ctx, chathooks.MetricTtsStart, ts, nil)
}

func (s *ChatSession) TraceTtsFirstFrame(ctx context.Context, ts int64) {
	s.emitMetricStage(ctx, chathooks.MetricTtsFirstFrame, ts, nil)
}

func (s *ChatSession) TraceTtsStop(ctx context.Context, ts int64, err error) {
	s.emitMetricStage(ctx, chathooks.MetricTtsStop, ts, err)
}
