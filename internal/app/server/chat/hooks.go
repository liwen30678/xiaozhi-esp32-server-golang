package chat

import (
	"context"
	"fmt"

	"github.com/cloudwego/eino/schema"
	domainhooks "xiaozhi-esp32-server-golang/internal/domain/hooks"
	"xiaozhi-esp32-server-golang/internal/domain/speaker"
)

const (
	eventASROutput      = "chat.asr.output"
	eventLLMInput       = "chat.llm.input"
	eventLLMOutput      = "chat.llm.output"
	eventTTSInput       = "chat.tts.input"
	eventTTSOutputStart = "chat.tts.output.start"
	eventTTSOutputStop  = "chat.tts.output.stop"
	eventMetric         = "chat.metric"
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

type ASROutputSyncHook func(HookContext, ASROutputData) (ASROutputData, bool, error)
type ASROutputAsyncHook func(HookContext, ASROutputData)

type LLMInputSyncHook func(HookContext, LLMInputData) (LLMInputData, bool, error)
type LLMInputAsyncHook func(HookContext, LLMInputData)

type LLMOutputSyncHook func(HookContext, LLMOutputData) (LLMOutputData, bool, error)
type LLMOutputAsyncHook func(HookContext, LLMOutputData)

type TTSInputSyncHook func(HookContext, TTSInputData) (TTSInputData, bool, error)
type TTSInputAsyncHook func(HookContext, TTSInputData)

type TTSOutputStartSyncHook func(HookContext, TTSOutputStartData) (TTSOutputStartData, bool, error)
type TTSOutputStartAsyncHook func(HookContext, TTSOutputStartData)

type TTSOutputStopSyncHook func(HookContext, TTSOutputStopData) (TTSOutputStopData, bool, error)
type TTSOutputStopAsyncHook func(HookContext, TTSOutputStopData)

type MetricSyncHook func(HookContext, MetricData) (MetricData, bool, error)
type MetricAsyncHook func(HookContext, MetricData)

type HookHub struct {
	hub *domainhooks.Hub
}

func NewHookHub() *HookHub {
	return &HookHub{hub: domainhooks.NewHub()}
}

var globalHookHub = NewHookHub()

func GlobalHookHub() *HookHub { return globalHookHub }

type PluginHooks struct {
	Name     string
	Priority int

	ASROutputSync  ASROutputSyncHook
	ASROutputAsync ASROutputAsyncHook
	LLMInputSync   LLMInputSyncHook
	LLMInputAsync  LLMInputAsyncHook
	LLMOutputSync  LLMOutputSyncHook
	LLMOutputAsync LLMOutputAsyncHook
	TTSInputSync   TTSInputSyncHook
	TTSInputAsync  TTSInputAsyncHook

	TTSOutputStartSync  TTSOutputStartSyncHook
	TTSOutputStartAsync TTSOutputStartAsyncHook
	TTSOutputStopSync   TTSOutputStopSyncHook
	TTSOutputStopAsync  TTSOutputStopAsyncHook
	MetricSync          MetricSyncHook
	MetricAsync         MetricAsyncHook
}

func AddPluginHooks(p PluginHooks) {
	if p.ASROutputSync != nil {
		AddASROutputSyncHook(p.Name, p.Priority, p.ASROutputSync)
	}
	if p.ASROutputAsync != nil {
		AddASROutputAsyncHook(p.Name, p.Priority, p.ASROutputAsync)
	}
	if p.LLMInputSync != nil {
		AddLLMInputSyncHook(p.Name, p.Priority, p.LLMInputSync)
	}
	if p.LLMInputAsync != nil {
		AddLLMInputAsyncHook(p.Name, p.Priority, p.LLMInputAsync)
	}
	if p.LLMOutputSync != nil {
		AddLLMOutputSyncHook(p.Name, p.Priority, p.LLMOutputSync)
	}
	if p.LLMOutputAsync != nil {
		AddLLMOutputAsyncHook(p.Name, p.Priority, p.LLMOutputAsync)
	}
	if p.TTSInputSync != nil {
		AddTTSInputSyncHook(p.Name, p.Priority, p.TTSInputSync)
	}
	if p.TTSInputAsync != nil {
		AddTTSInputAsyncHook(p.Name, p.Priority, p.TTSInputAsync)
	}
	if p.TTSOutputStartSync != nil {
		AddTTSOutputStartSyncHook(p.Name, p.Priority, p.TTSOutputStartSync)
	}
	if p.TTSOutputStartAsync != nil {
		AddTTSOutputStartAsyncHook(p.Name, p.Priority, p.TTSOutputStartAsync)
	}
	if p.TTSOutputStopSync != nil {
		AddTTSOutputStopSyncHook(p.Name, p.Priority, p.TTSOutputStopSync)
	}
	if p.TTSOutputStopAsync != nil {
		AddTTSOutputStopAsyncHook(p.Name, p.Priority, p.TTSOutputStopAsync)
	}
	if p.MetricSync != nil {
		AddMetricSyncHook(p.Name, p.Priority, p.MetricSync)
	}
	if p.MetricAsync != nil {
		AddMetricAsyncHook(p.Name, p.Priority, p.MetricAsync)
	}
}

func toDomainCtx(ctx HookContext) domainhooks.Context {
	return domainhooks.Context{
		Ctx: ctx.Ctx,
		Meta: map[string]any{
			"session":    ctx.Session,
			"session_id": ctx.SessionID,
			"device_id":  ctx.DeviceID,
		},
	}
}

func fromDomainCtx(ctx domainhooks.Context) HookContext {
	hctx := HookContext{Ctx: ctx.Ctx}
	if ctx.Meta == nil {
		return hctx
	}
	if s, ok := ctx.Meta["session"].(*ChatSession); ok {
		hctx.Session = s
	}
	if sid, ok := ctx.Meta["session_id"].(string); ok {
		hctx.SessionID = sid
	}
	if did, ok := ctx.Meta["device_id"].(string); ok {
		hctx.DeviceID = did
	}
	return hctx
}

func emitTyped[T any](h *HookHub, event string, hctx HookContext, in T) (T, bool, error) {
	out, stop, err := h.hub.Emit(event, toDomainCtx(hctx), in)
	if err != nil {
		return in, stop, err
	}
	typed, ok := out.(T)
	if !ok {
		return in, stop, fmt.Errorf("hook output type mismatch for event %s", event)
	}
	return typed, stop, nil
}

func (h *HookHub) RunASROutput(hctx HookContext, in ASROutputData) (ASROutputData, bool, error) {
	return emitTyped(h, eventASROutput, hctx, in)
}

func (h *HookHub) RunLLMInput(hctx HookContext, in LLMInputData) (LLMInputData, bool, error) {
	return emitTyped(h, eventLLMInput, hctx, in)
}

func (h *HookHub) RunLLMOutput(hctx HookContext, in LLMOutputData) (LLMOutputData, bool, error) {
	return emitTyped(h, eventLLMOutput, hctx, in)
}

func (h *HookHub) RunTTSInput(hctx HookContext, in TTSInputData) (TTSInputData, bool, error) {
	return emitTyped(h, eventTTSInput, hctx, in)
}

func (h *HookHub) RunTTSOutputStart(hctx HookContext, in TTSOutputStartData) (TTSOutputStartData, bool, error) {
	return emitTyped(h, eventTTSOutputStart, hctx, in)
}

func (h *HookHub) RunTTSOutputStop(hctx HookContext, in TTSOutputStopData) (TTSOutputStopData, bool, error) {
	return emitTyped(h, eventTTSOutputStop, hctx, in)
}

func (h *HookHub) RunMetric(hctx HookContext, in MetricData) (MetricData, bool, error) {
	return emitTyped(h, eventMetric, hctx, in)
}

func AddASROutputSyncHook(name string, priority int, hook ASROutputSyncHook) {
	globalHookHub.hub.RegisterSync(eventASROutput, name, priority, func(ctx domainhooks.Context, payload any) (any, bool, error) {
		in, ok := payload.(ASROutputData)
		if !ok {
			return payload, false, fmt.Errorf("invalid ASR_OUTPUT payload")
		}
		return hook(fromDomainCtx(ctx), in)
	})
}

func AddASROutputAsyncHook(name string, priority int, hook ASROutputAsyncHook) {
	globalHookHub.hub.RegisterAsync(eventASROutput, name, priority, func(ctx domainhooks.Context, payload any) {
		in, ok := payload.(ASROutputData)
		if !ok {
			return
		}
		hook(fromDomainCtx(ctx), in)
	})
}

func AddLLMInputSyncHook(name string, priority int, hook LLMInputSyncHook) {
	globalHookHub.hub.RegisterSync(eventLLMInput, name, priority, func(ctx domainhooks.Context, payload any) (any, bool, error) {
		in, ok := payload.(LLMInputData)
		if !ok {
			return payload, false, fmt.Errorf("invalid LLM_INPUT payload")
		}
		return hook(fromDomainCtx(ctx), in)
	})
}

func AddLLMInputAsyncHook(name string, priority int, hook LLMInputAsyncHook) {
	globalHookHub.hub.RegisterAsync(eventLLMInput, name, priority, func(ctx domainhooks.Context, payload any) {
		in, ok := payload.(LLMInputData)
		if !ok {
			return
		}
		hook(fromDomainCtx(ctx), in)
	})
}

func AddLLMOutputSyncHook(name string, priority int, hook LLMOutputSyncHook) {
	globalHookHub.hub.RegisterSync(eventLLMOutput, name, priority, func(ctx domainhooks.Context, payload any) (any, bool, error) {
		in, ok := payload.(LLMOutputData)
		if !ok {
			return payload, false, fmt.Errorf("invalid LLM_OUTPUT payload")
		}
		return hook(fromDomainCtx(ctx), in)
	})
}

func AddLLMOutputAsyncHook(name string, priority int, hook LLMOutputAsyncHook) {
	globalHookHub.hub.RegisterAsync(eventLLMOutput, name, priority, func(ctx domainhooks.Context, payload any) {
		in, ok := payload.(LLMOutputData)
		if !ok {
			return
		}
		hook(fromDomainCtx(ctx), in)
	})
}

func AddTTSInputSyncHook(name string, priority int, hook TTSInputSyncHook) {
	globalHookHub.hub.RegisterSync(eventTTSInput, name, priority, func(ctx domainhooks.Context, payload any) (any, bool, error) {
		in, ok := payload.(TTSInputData)
		if !ok {
			return payload, false, fmt.Errorf("invalid TTS_INPUT payload")
		}
		return hook(fromDomainCtx(ctx), in)
	})
}

func AddTTSInputAsyncHook(name string, priority int, hook TTSInputAsyncHook) {
	globalHookHub.hub.RegisterAsync(eventTTSInput, name, priority, func(ctx domainhooks.Context, payload any) {
		in, ok := payload.(TTSInputData)
		if !ok {
			return
		}
		hook(fromDomainCtx(ctx), in)
	})
}

func AddTTSOutputStartSyncHook(name string, priority int, hook TTSOutputStartSyncHook) {
	globalHookHub.hub.RegisterSync(eventTTSOutputStart, name, priority, func(ctx domainhooks.Context, payload any) (any, bool, error) {
		in, ok := payload.(TTSOutputStartData)
		if !ok {
			return payload, false, fmt.Errorf("invalid TTS_OUTPUT_START payload")
		}
		return hook(fromDomainCtx(ctx), in)
	})
}

func AddTTSOutputStartAsyncHook(name string, priority int, hook TTSOutputStartAsyncHook) {
	globalHookHub.hub.RegisterAsync(eventTTSOutputStart, name, priority, func(ctx domainhooks.Context, payload any) {
		in, ok := payload.(TTSOutputStartData)
		if !ok {
			return
		}
		hook(fromDomainCtx(ctx), in)
	})
}

func AddTTSOutputStopSyncHook(name string, priority int, hook TTSOutputStopSyncHook) {
	globalHookHub.hub.RegisterSync(eventTTSOutputStop, name, priority, func(ctx domainhooks.Context, payload any) (any, bool, error) {
		in, ok := payload.(TTSOutputStopData)
		if !ok {
			return payload, false, fmt.Errorf("invalid TTS_OUTPUT_STOP payload")
		}
		return hook(fromDomainCtx(ctx), in)
	})
}

func AddTTSOutputStopAsyncHook(name string, priority int, hook TTSOutputStopAsyncHook) {
	globalHookHub.hub.RegisterAsync(eventTTSOutputStop, name, priority, func(ctx domainhooks.Context, payload any) {
		in, ok := payload.(TTSOutputStopData)
		if !ok {
			return
		}
		hook(fromDomainCtx(ctx), in)
	})
}

func AddMetricSyncHook(name string, priority int, hook MetricSyncHook) {
	globalHookHub.hub.RegisterSync(eventMetric, name, priority, func(ctx domainhooks.Context, payload any) (any, bool, error) {
		in, ok := payload.(MetricData)
		if !ok {
			return payload, false, fmt.Errorf("invalid METRIC payload")
		}
		return hook(fromDomainCtx(ctx), in)
	})
}

func AddMetricAsyncHook(name string, priority int, hook MetricAsyncHook) {
	globalHookHub.hub.RegisterAsync(eventMetric, name, priority, func(ctx domainhooks.Context, payload any) {
		in, ok := payload.(MetricData)
		if !ok {
			return
		}
		hook(fromDomainCtx(ctx), in)
	})
}
