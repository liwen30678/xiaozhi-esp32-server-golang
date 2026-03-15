package hooks

import (
	"context"
	"fmt"
	"runtime"
	"sort"
	"sync"
)

type Context struct {
	Ctx  context.Context
	Meta map[string]any
}

type SyncHandler func(Context, any) (any, bool, error)
type AsyncHandler func(Context, any)

type namedSync struct {
	name     string
	priority int
	handler  SyncHandler
}

type namedAsync struct {
	name     string
	priority int
	handler  AsyncHandler
}

type Hub struct {
	mu sync.RWMutex

	syncHandlers  map[string][]namedSync
	asyncHandlers map[string][]namedAsync

	asyncTasks chan func()
}

func NewHub() *Hub {
	h := &Hub{
		syncHandlers:  make(map[string][]namedSync),
		asyncHandlers: make(map[string][]namedAsync),
		asyncTasks:    make(chan func(), 256),
	}
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

func (h *Hub) RegisterSync(event, name string, priority int, handler SyncHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cur := h.syncHandlers[event]
	next := make([]namedSync, 0, len(cur)+1)
	next = append(next, cur...)
	next = append(next, namedSync{name: name, priority: priority, handler: handler})
	sort.SliceStable(next, func(i, j int) bool { return next[i].priority < next[j].priority })
	h.syncHandlers[event] = next
}

func (h *Hub) RegisterAsync(event, name string, priority int, handler AsyncHandler) {
	h.mu.Lock()
	defer h.mu.Unlock()
	cur := h.asyncHandlers[event]
	next := make([]namedAsync, 0, len(cur)+1)
	next = append(next, cur...)
	next = append(next, namedAsync{name: name, priority: priority, handler: handler})
	sort.SliceStable(next, func(i, j int) bool { return next[i].priority < next[j].priority })
	h.asyncHandlers[event] = next
}

func (h *Hub) Emit(event string, ctx Context, payload any) (any, bool, error) {
	h.mu.RLock()
	syncs := h.syncHandlers[event]
	asyncs := h.asyncHandlers[event]
	h.mu.RUnlock()

	out := payload
	for _, hk := range syncs {
		next, stop, err := hk.handler(ctx, out)
		if err != nil {
			return out, stop, fmt.Errorf("hook %s failed: %w", hk.name, err)
		}
		out = next
		if stop {
			h.emitAsync(ctx, asyncs, out)
			return out, true, nil
		}
	}
	h.emitAsync(ctx, asyncs, out)
	return out, false, nil
}

func (h *Hub) emitAsync(ctx Context, hooks []namedAsync, payload any) {
	for _, hk := range hooks {
		handler := hk.handler
		c := ctx
		p := payload
		select {
		case h.asyncTasks <- func() { handler(c, p) }:
		default:
		}
	}
}
