package chat

import (
	"fmt"
	"sync"
	"time"

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
}

var (
	statPluginOnce sync.Once
	statPluginInst = &statisticPlugin{
		currentTurn: make(map[string]int64),
		turns:       make(map[string]*turnMetric),
	}
)

func ensureStatisticPluginRegistered() {
	statPluginOnce.Do(func() {
		AddMetricAsyncHook("statistic_plugin", 100, statPluginInst.onMetric)
	})
}

func (p *statisticPlugin) onMetric(ctx HookContext, data MetricData) {
	if ctx.SessionID == "" {
		return
	}
	p.mu.Lock()
	defer p.mu.Unlock()

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
		delete(p.turns, ctx.SessionID)
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

	// 简单清理，避免异常场景积压
	now := time.Now().UnixMilli()
	if now-tm.turnStartTs > 5*60*1000 {
		return
	}
}
