package chat

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"

	"github.com/cloudwego/eino/schema"
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

type namedSyncHook[T any] struct {
	name     string
	priority int
	hook     func(HookContext, T) (T, bool, error)
}

type namedAsyncHook[T any] struct {
	name     string
	priority int
	hook     func(HookContext, T)
}

type HookHub struct {
	mu sync.RWMutex

	asrOutputSync  []namedSyncHook[ASROutputData]
	asrOutputAsync []namedAsyncHook[ASROutputData]

	llmInputSync  []namedSyncHook[LLMInputData]
	llmInputAsync []namedAsyncHook[LLMInputData]

	llmOutputSync  []namedSyncHook[LLMOutputData]
	llmOutputAsync []namedAsyncHook[LLMOutputData]

	ttsInputSync  []namedSyncHook[TTSInputData]
	ttsInputAsync []namedAsyncHook[TTSInputData]

	ttsOutputStartSync  []namedSyncHook[TTSOutputStartData]
	ttsOutputStartAsync []namedAsyncHook[TTSOutputStartData]

	ttsOutputStopSync  []namedSyncHook[TTSOutputStopData]
	ttsOutputStopAsync []namedAsyncHook[TTSOutputStopData]

	asyncTasks chan func()
}

func NewHookHub() *HookHub {
	h := &HookHub{asyncTasks: make(chan func(), 256)}
	workers := runtime.NumCPU() / 2
	if workers < 2 {
		workers = 2
	}
	for i := 0; i < workers; i++ {
		go func() {
			for task := range h.asyncTasks {
				if task != nil {
					task()
				}
			}
		}()
	}
	return h
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
}

func runSyncHooks[T any](hooks []namedSyncHook[T], hctx HookContext, data T) (T, bool, error) {
	for _, hk := range hooks {
		out, stop, err := hk.hook(hctx, data)
		if err != nil {
			return data, stop, fmt.Errorf("hook %s failed: %w", hk.name, err)
		}
		data = out
		if stop {
			return data, true, nil
		}
	}
	return data, false, nil
}

func emitAsyncHooks[T any](asyncTasks chan func(), hooks []namedAsyncHook[T], hctx HookContext, data T) {
	for _, hk := range hooks {
		hookFn := hk.hook
		d := data
		c := hctx
		select {
		case asyncTasks <- func() { hookFn(c, d) }:
		default:
		}
	}
}

func (h *HookHub) RunASROutput(hctx HookContext, in ASROutputData) (ASROutputData, bool, error) {
	h.mu.RLock()
	syncHooks := h.asrOutputSync
	asyncHooks := h.asrOutputAsync
	h.mu.RUnlock()
	out, stop, err := runSyncHooks(syncHooks, hctx, in)
	emitAsyncHooks(h.asyncTasks, asyncHooks, hctx, out)
	return out, stop, err
}

func (h *HookHub) RunLLMInput(hctx HookContext, in LLMInputData) (LLMInputData, bool, error) {
	h.mu.RLock()
	syncHooks := h.llmInputSync
	asyncHooks := h.llmInputAsync
	h.mu.RUnlock()
	out, stop, err := runSyncHooks(syncHooks, hctx, in)
	emitAsyncHooks(h.asyncTasks, asyncHooks, hctx, out)
	return out, stop, err
}

func (h *HookHub) RunLLMOutput(hctx HookContext, in LLMOutputData) (LLMOutputData, bool, error) {
	h.mu.RLock()
	syncHooks := h.llmOutputSync
	asyncHooks := h.llmOutputAsync
	h.mu.RUnlock()
	out, stop, err := runSyncHooks(syncHooks, hctx, in)
	emitAsyncHooks(h.asyncTasks, asyncHooks, hctx, out)
	return out, stop, err
}

func (h *HookHub) RunTTSInput(hctx HookContext, in TTSInputData) (TTSInputData, bool, error) {
	h.mu.RLock()
	syncHooks := h.ttsInputSync
	asyncHooks := h.ttsInputAsync
	h.mu.RUnlock()
	out, stop, err := runSyncHooks(syncHooks, hctx, in)
	emitAsyncHooks(h.asyncTasks, asyncHooks, hctx, out)
	return out, stop, err
}

func (h *HookHub) RunTTSOutputStart(hctx HookContext, in TTSOutputStartData) (TTSOutputStartData, bool, error) {
	h.mu.RLock()
	syncHooks := h.ttsOutputStartSync
	asyncHooks := h.ttsOutputStartAsync
	h.mu.RUnlock()
	out, stop, err := runSyncHooks(syncHooks, hctx, in)
	emitAsyncHooks(h.asyncTasks, asyncHooks, hctx, out)
	return out, stop, err
}

func (h *HookHub) RunTTSOutputStop(hctx HookContext, in TTSOutputStopData) (TTSOutputStopData, bool, error) {
	h.mu.RLock()
	syncHooks := h.ttsOutputStopSync
	asyncHooks := h.ttsOutputStopAsync
	h.mu.RUnlock()
	out, stop, err := runSyncHooks(syncHooks, hctx, in)
	emitAsyncHooks(h.asyncTasks, asyncHooks, hctx, out)
	return out, stop, err
}

func sortSync[T any](hooks []namedSyncHook[T]) {
	sort.SliceStable(hooks, func(i, j int) bool { return hooks[i].priority < hooks[j].priority })
}

func sortAsync[T any](hooks []namedAsyncHook[T]) {
	sort.SliceStable(hooks, func(i, j int) bool { return hooks[i].priority < hooks[j].priority })
}

func AddASROutputSyncHook(name string, priority int, hook ASROutputSyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.asrOutputSync = appendSortedSyncHook(
		globalHookHub.asrOutputSync,
		namedSyncHook[ASROutputData]{name: name, priority: priority, hook: hook},
	)
}
func AddASROutputAsyncHook(name string, priority int, hook ASROutputAsyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.asrOutputAsync = appendSortedAsyncHook(
		globalHookHub.asrOutputAsync,
		namedAsyncHook[ASROutputData]{name: name, priority: priority, hook: hook},
	)
}
func AddLLMInputSyncHook(name string, priority int, hook LLMInputSyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.llmInputSync = appendSortedSyncHook(
		globalHookHub.llmInputSync,
		namedSyncHook[LLMInputData]{name: name, priority: priority, hook: hook},
	)
}
func AddLLMInputAsyncHook(name string, priority int, hook LLMInputAsyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.llmInputAsync = appendSortedAsyncHook(
		globalHookHub.llmInputAsync,
		namedAsyncHook[LLMInputData]{name: name, priority: priority, hook: hook},
	)
}
func AddLLMOutputSyncHook(name string, priority int, hook LLMOutputSyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.llmOutputSync = appendSortedSyncHook(
		globalHookHub.llmOutputSync,
		namedSyncHook[LLMOutputData]{name: name, priority: priority, hook: hook},
	)
}
func AddLLMOutputAsyncHook(name string, priority int, hook LLMOutputAsyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.llmOutputAsync = appendSortedAsyncHook(
		globalHookHub.llmOutputAsync,
		namedAsyncHook[LLMOutputData]{name: name, priority: priority, hook: hook},
	)
}
func AddTTSInputSyncHook(name string, priority int, hook TTSInputSyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.ttsInputSync = appendSortedSyncHook(
		globalHookHub.ttsInputSync,
		namedSyncHook[TTSInputData]{name: name, priority: priority, hook: hook},
	)
}
func AddTTSInputAsyncHook(name string, priority int, hook TTSInputAsyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.ttsInputAsync = appendSortedAsyncHook(
		globalHookHub.ttsInputAsync,
		namedAsyncHook[TTSInputData]{name: name, priority: priority, hook: hook},
	)
}
func AddTTSOutputStartSyncHook(name string, priority int, hook TTSOutputStartSyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.ttsOutputStartSync = appendSortedSyncHook(
		globalHookHub.ttsOutputStartSync,
		namedSyncHook[TTSOutputStartData]{name: name, priority: priority, hook: hook},
	)
}
func AddTTSOutputStartAsyncHook(name string, priority int, hook TTSOutputStartAsyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.ttsOutputStartAsync = appendSortedAsyncHook(
		globalHookHub.ttsOutputStartAsync,
		namedAsyncHook[TTSOutputStartData]{name: name, priority: priority, hook: hook},
	)
}
func AddTTSOutputStopSyncHook(name string, priority int, hook TTSOutputStopSyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.ttsOutputStopSync = appendSortedSyncHook(
		globalHookHub.ttsOutputStopSync,
		namedSyncHook[TTSOutputStopData]{name: name, priority: priority, hook: hook},
	)
}
func AddTTSOutputStopAsyncHook(name string, priority int, hook TTSOutputStopAsyncHook) {
	globalHookHub.mu.Lock()
	defer globalHookHub.mu.Unlock()
	globalHookHub.ttsOutputStopAsync = appendSortedAsyncHook(
		globalHookHub.ttsOutputStopAsync,
		namedAsyncHook[TTSOutputStopData]{name: name, priority: priority, hook: hook},
	)
}

func appendSortedSyncHook[T any](src []namedSyncHook[T], item namedSyncHook[T]) []namedSyncHook[T] {
	dst := make([]namedSyncHook[T], 0, len(src)+1)
	dst = append(dst, src...)
	dst = append(dst, item)
	sortSync(dst)
	return dst
}

func appendSortedAsyncHook[T any](src []namedAsyncHook[T], item namedAsyncHook[T]) []namedAsyncHook[T] {
	dst := make([]namedAsyncHook[T], 0, len(src)+1)
	dst = append(dst, src...)
	dst = append(dst, item)
	sortAsync(dst)
	return dst
}
