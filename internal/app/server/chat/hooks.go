package chat

import (
	"context"

	"github.com/cloudwego/eino/schema"
	domainhooks "xiaozhi-esp32-server-golang/internal/domain/hooks"
	"xiaozhi-esp32-server-golang/internal/domain/speaker"
)

type HookContext struct {
	Ctx       context.Context
	Session   *ChatSession
	SessionID string
	DeviceID  string
}

type ASROutputData struct {
	Text          string
	SpeakerResult *speaker.IdentifyResult
}

type LLMInputData struct {
	UserMessage     *schema.Message
	RequestMessages []*schema.Message
	Tools           []*schema.ToolInfo
}

type LLMOutputData struct {
	FullText string
	Err      error
}

type TTSInputData struct {
	Text    string
	IsStart bool
	IsEnd   bool
}

type TTSOutputStartData struct{}

type TTSOutputStopData struct {
	Err error
}

type MetricStage string

const (
	MetricTurnStart     MetricStage = "turn_start"
	MetricAsrFirstText  MetricStage = "asr_first_text"
	MetricAsrFinalText  MetricStage = "asr_final_text"
	MetricLlmStart      MetricStage = "llm_start"
	MetricLlmFirstToken MetricStage = "llm_first_token"
	MetricLlmEnd        MetricStage = "llm_end"
	MetricTtsStart      MetricStage = "tts_start"
	MetricTtsFirstFrame MetricStage = "tts_first_frame"
	MetricTtsStop       MetricStage = "tts_stop"
)

type MetricData struct {
	Stage MetricStage
	Ts    int64
	Err   error
}

type HookHub struct {
	hub *domainhooks.Hub
}

func NewHookHub() *HookHub {
	return &HookHub{hub: domainhooks.NewHub()}
}

var globalHookHub = NewHookHub()

func GlobalHookHub() *HookHub { return globalHookHub }

func toDomainCtx(ctx HookContext) domainhooks.Context {
	return domainhooks.Context{
		Ctx: ctx.Ctx,
		Meta: map[string]any{
			domainhooks.MetaSession:   ctx.Session,
			domainhooks.MetaSessionID: ctx.SessionID,
			domainhooks.MetaDeviceID:  ctx.DeviceID,
		},
	}
}

func (h *HookHub) Emit(event string, hctx HookContext, payload any) (any, bool, error) {
	return h.hub.Emit(event, toDomainCtx(hctx), payload)
}

func (h *HookHub) RegisterSync(event, name string, priority int, handler domainhooks.SyncHandler) {
	h.hub.RegisterSync(event, name, priority, handler)
}

func (h *HookHub) RegisterAsync(event, name string, priority int, handler domainhooks.AsyncHandler) {
	h.hub.RegisterAsync(event, name, priority, handler)
}
