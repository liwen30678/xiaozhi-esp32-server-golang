package chat

import (
	"fmt"
	"sync"
	"time"

	domainhooks "xiaozhi-esp32-server-golang/internal/domain/hooks"
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
	// sessionID -> current turn
	currentTurn map[string]int64
	// sessionID -> turn metrics
	turns map[string]*turnMetric
	// sessionID -> last seen ts
	lastSeen map[string]int64
}

var (
	statPluginOnce sync.Once
	statPluginInst = &statisticPlugin{
		currentTurn: make(map[string]int64),
		turns:       make(map[string]*turnMetric),
		lastSeen:    make(map[string]int64),
	}
)

func ensureStatisticPluginRegistered() {
	statPluginOnce.Do(func() {
		GlobalHookHub().RegisterAsync(domainhooks.EventChatMetric, "statistic_plugin", 100, statPluginInst.onMetric)
	})
}

func (p *statisticPlugin) onMetric(ctx domainhooks.Context, payload any) {
	data, ok := payload.(MetricData)
	if !ok {
		return
	}
	sessionID, _ := ctx.Meta[domainhooks.MetaSessionID].(string)
	if sessionID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

	nowTs := time.Now().UnixMilli()
	p.lastSeen[sessionID] = nowTs
	p.cleanupStale(nowTs)

	tm := p.getOrCreateTurn(sessionID, data.Stage)
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
		p.logTurnMetric(sessionID, tm)
		delete(p.turns, sessionID)
	}
}

func (p *statisticPlugin) getOrCreateTurn(sessionID string, stage MetricStage) *turnMetric {
	if stage == MetricTurnStart {
		p.currentTurn[sessionID]++
		tm := &turnMetric{turnID: p.currentTurn[sessionID]}
		p.turns[sessionID] = tm
		return tm
	}
	if tm, ok := p.turns[sessionID]; ok {
		return tm
	}
	// 兜底：若缺失 turn_start，按当前 turn 建立一个
	if p.currentTurn[sessionID] == 0 {
		p.currentTurn[sessionID] = 1
	}
	tm := &turnMetric{turnID: p.currentTurn[sessionID]}
	p.turns[sessionID] = tm
	return tm
}

func calcDelta(start, end int64) int64 {
	if start <= 0 || end <= 0 || end < start {
		return 0
	}
	return end - start
}

func (p *statisticPlugin) logTurnMetric(sessionID string, tm *turnMetric) {
	msg := fmt.Sprintf(
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
	log.Infof(msg)

}

func (p *statisticPlugin) cleanupStale(nowTs int64) {
	const ttl = int64(2 * 60 * 1000)
	for sessionID, last := range p.lastSeen {
		if nowTs-last > ttl {
			delete(p.lastSeen, sessionID)
			delete(p.turns, sessionID)
		}
	}
}
