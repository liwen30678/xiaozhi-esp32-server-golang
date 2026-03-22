package hooks

import (
	"context"
	"sync"
	"time"

	cmap "github.com/orcaman/concurrent-map/v2"
	log "xiaozhi-esp32-server-golang/logger"
)

type turnMetric struct {
	turnID int64

	turnStartTs     int64
	asrFirstTextTs  int64
	asrFinalTextTs  int64
	llmStartTs      int64
	llmFirstTokenTs int64
	llmEndTs        int64
	ttsStartTs      int64
	ttsFirstFrameTs int64
	ttsStopTs       int64
}

type statisticPlugin struct {
	mu sync.Mutex

	currentTurn cmap.ConcurrentMap[string, int64]
	turns       cmap.ConcurrentMap[string, *turnMetric]
	lastSeen    cmap.ConcurrentMap[string, int64]

	cleanupCounter   int64
	cleanupThreshold int64
}

func newStatisticPlugin() *statisticPlugin {
	return &statisticPlugin{
		currentTurn:      cmap.New[int64](),
		turns:            cmap.New[*turnMetric](),
		lastSeen:         cmap.New[int64](),
		cleanupThreshold: 100,
	}
}

func (p *statisticPlugin) Init(context.Context) error { return nil }
func (p *statisticPlugin) Close() error               { return nil }

func BuiltinRegistrations() []Registration {
	plugin := newStatisticPlugin()
	meta := PluginMeta{
		Name:        "statistic_plugin",
		Version:     "v1",
		Description: "Aggregate turn metrics and log a summary on TTS stop",
		Priority:    100,
		Enabled:     true,
		Kind:        PluginKindObserver,
		Stage:       EventChatMetric,
	}
	return []Registration{{
		Meta:      meta,
		Lifecycle: plugin,
		Register: func(hub *Hub, meta PluginMeta) error {
			return hub.RegisterObserver(EventChatMetric, meta, plugin.onMetric)
		},
	}}
}

func (p *statisticPlugin) onMetric(ctx Context, payload any) {
	data, ok := payload.(MetricData)
	if !ok || ctx.SessionID == "" {
		return
	}

	p.mu.Lock()
	nowTs := time.Now().UnixMilli()
	p.lastSeen.Set(ctx.SessionID, nowTs)
	if p.cleanupCounter++; p.cleanupCounter%p.cleanupThreshold == 0 {
		p.cleanupStale(nowTs)
	}

	tm := p.getOrCreateTurnLocked(ctx.SessionID)
	switch data.Stage {
	case MetricTurnStart:
		if tm.turnStartTs == 0 {
			tm.turnStartTs = data.Ts
		}
	case MetricAsrFirstText:
		if tm.asrFirstTextTs == 0 {
			tm.asrFirstTextTs = data.Ts
		}
	case MetricAsrFinalText:
		if tm.asrFinalTextTs == 0 {
			tm.asrFinalTextTs = data.Ts
		}
	case MetricLlmStart:
		if tm.llmStartTs == 0 {
			tm.llmStartTs = data.Ts
		}
	case MetricLlmFirstToken:
		if tm.llmFirstTokenTs == 0 {
			tm.llmFirstTokenTs = data.Ts
		}
	case MetricLlmEnd:
		if tm.llmEndTs == 0 {
			tm.llmEndTs = data.Ts
		}
	case MetricTtsStart:
		if tm.ttsStartTs == 0 {
			tm.ttsStartTs = data.Ts
		}
	case MetricTtsFirstFrame:
		if tm.ttsFirstFrameTs == 0 {
			tm.ttsFirstFrameTs = data.Ts
		}
	case MetricTtsStop:
		if tm.ttsStopTs == 0 {
			tm.ttsStopTs = data.Ts
		}
	}

	var completed *turnMetric
	if data.Stage == MetricTtsStop {
		snapshot := *tm
		completed = &snapshot
		p.turns.Remove(ctx.SessionID)
	}
	p.mu.Unlock()

	if completed != nil {
		p.logTurnMetric(ctx.SessionID, completed)
	}
}

func (p *statisticPlugin) getOrCreateTurnLocked(sessionID string) *turnMetric {
	if tm, ok := p.turns.Get(sessionID); ok {
		return tm
	}

	newTurnID := int64(1)
	if val, ok := p.currentTurn.Get(sessionID); ok {
		newTurnID = val + 1
	}
	p.currentTurn.Set(sessionID, newTurnID)

	tm := &turnMetric{turnID: newTurnID}
	p.turns.Set(sessionID, tm)
	return tm
}

func calcDelta(start, end int64) int64 {
	if start <= 0 || end <= 0 || end < start {
		return 0
	}
	return end - start
}

func (p *statisticPlugin) logTurnMetric(sessionID string, tm *turnMetric) {
	log.Infof(
		"metric turn=%d session=%s asr_first=%dms asr_final=%dms llm_first=%dms llm_total=%dms tts_first=%dms tts_total=%dms e2e_first=%dms e2e_total=%dms",
		tm.turnID,
		sessionID,
		calcDelta(tm.turnStartTs, tm.asrFirstTextTs),
		calcDelta(tm.turnStartTs, tm.asrFinalTextTs),
		calcDelta(tm.llmStartTs, tm.llmFirstTokenTs),
		calcDelta(tm.llmStartTs, tm.llmEndTs),
		calcDelta(tm.ttsStartTs, tm.ttsFirstFrameTs),
		calcDelta(tm.ttsStartTs, tm.ttsStopTs),
		calcDelta(tm.turnStartTs, tm.ttsFirstFrameTs),
		calcDelta(tm.turnStartTs, tm.ttsStopTs),
	)
}

func (p *statisticPlugin) cleanupStale(nowTs int64) {
	const ttl = int64(2 * 60 * 1000)
	keysToDelete := make([]string, 0)
	p.lastSeen.IterCb(func(key string, value int64) {
		if nowTs-value > ttl {
			keysToDelete = append(keysToDelete, key)
		}
	})
	for _, key := range keysToDelete {
		p.lastSeen.Remove(key)
		p.turns.Remove(key)
		p.currentTurn.Remove(key)
	}
}
