package hooks

import (
	"context"
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

	nowTs := time.Now().UnixMilli()
	p.lastSeen.Set(ctx.SessionID, nowTs)
	if p.cleanupCounter++; p.cleanupCounter%p.cleanupThreshold == 0 {
		p.cleanupStale(nowTs)
	}

	tm := p.getOrCreateTurn(ctx.SessionID, data.Stage)
	switch data.Stage {
	case MetricTurnStart:
		tm.turnStartTs = data.Ts
	case MetricAsrFirstText:
		tm.asrFirstTextTs = data.Ts
	case MetricAsrFinalText:
		tm.asrFinalTextTs = data.Ts
	case MetricLlmStart:
		tm.llmStartTs = data.Ts
	case MetricLlmFirstToken:
		tm.llmFirstTokenTs = data.Ts
	case MetricLlmEnd:
		tm.llmEndTs = data.Ts
	case MetricTtsStart:
		tm.ttsStartTs = data.Ts
	case MetricTtsFirstFrame:
		tm.ttsFirstFrameTs = data.Ts
	case MetricTtsStop:
		tm.ttsStopTs = data.Ts
		p.logTurnMetric(ctx.SessionID, tm)
		p.turns.Remove(ctx.SessionID)
	}
}

func (p *statisticPlugin) getOrCreateTurn(sessionID string, stage MetricStage) *turnMetric {
	if stage == MetricTurnStart {
		var newTurnID int64 = 1
		if val, ok := p.currentTurn.Get(sessionID); ok {
			newTurnID = val + 1
		}
		p.currentTurn.Set(sessionID, newTurnID)

		tm := &turnMetric{turnID: newTurnID}
		p.turns.Set(sessionID, tm)
		return tm
	}
	if val, ok := p.turns.Get(sessionID); ok {
		return val
	}
	var turnID int64 = 1
	if val, ok := p.currentTurn.Get(sessionID); ok {
		turnID = val
	} else {
		p.currentTurn.Set(sessionID, turnID)
	}
	tm := &turnMetric{turnID: turnID}
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
