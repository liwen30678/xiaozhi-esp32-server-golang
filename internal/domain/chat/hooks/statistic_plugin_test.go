package hooks

import (
	"context"
	"testing"
)

func testHookContext(sessionID string) Context {
	return Context{
		Ctx:       context.Background(),
		SessionID: sessionID,
		DeviceID:  "device-test",
	}
}

func TestStatisticPluginHandlesLateTurnStart(t *testing.T) {
	plugin := newStatisticPlugin()
	ctx := testHookContext("session-late-start")

	plugin.onMetric(ctx, MetricData{Stage: MetricAsrFirstText, Ts: 20})
	plugin.onMetric(ctx, MetricData{Stage: MetricTurnStart, Ts: 10})
	plugin.onMetric(ctx, MetricData{Stage: MetricTtsFirstFrame, Ts: 80})

	tm, ok := plugin.turns.Get(ctx.SessionID)
	if !ok {
		t.Fatalf("expected active turn for session %q", ctx.SessionID)
	}
	if tm.turnID != 1 {
		t.Fatalf("turnID = %d, want 1", tm.turnID)
	}
	if tm.turnStartTs != 10 {
		t.Fatalf("turnStartTs = %d, want 10", tm.turnStartTs)
	}
	if tm.asrFirstTextTs != 20 {
		t.Fatalf("asrFirstTextTs = %d, want 20", tm.asrFirstTextTs)
	}
}

func TestStatisticPluginClosesTurnOnTtsStop(t *testing.T) {
	plugin := newStatisticPlugin()
	ctx := testHookContext("session-stop")

	plugin.onMetric(ctx, MetricData{Stage: MetricTurnStart, Ts: 10})
	plugin.onMetric(ctx, MetricData{Stage: MetricAsrFinalText, Ts: 20})
	plugin.onMetric(ctx, MetricData{Stage: MetricTtsStop, Ts: 30})

	if _, ok := plugin.turns.Get(ctx.SessionID); ok {
		t.Fatalf("expected turn to be removed after tts_stop")
	}
	currentTurn, ok := plugin.currentTurn.Get(ctx.SessionID)
	if !ok {
		t.Fatalf("expected currentTurn to be tracked")
	}
	if currentTurn != 1 {
		t.Fatalf("currentTurn = %d, want 1", currentTurn)
	}

	plugin.onMetric(ctx, MetricData{Stage: MetricAsrFirstText, Ts: 40})

	tm, ok := plugin.turns.Get(ctx.SessionID)
	if !ok {
		t.Fatalf("expected next turn to be created")
	}
	if tm.turnID != 2 {
		t.Fatalf("turnID = %d, want 2", tm.turnID)
	}
	if tm.asrFirstTextTs != 40 {
		t.Fatalf("asrFirstTextTs = %d, want 40", tm.asrFirstTextTs)
	}
}
