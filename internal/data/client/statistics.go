package client

import "time"

type Statistic struct {
	TurnStartTs     int64
	AsrFirstTextTs  int64
	AsrFinalTextTs  int64
	LlmStartTs      int64
	LlmFirstTokenTs int64
	LlmEndTs        int64
	TtsStartTs      int64
	TtsFirstFrameTs int64
	TtsStopTs       int64
}

func (s *Statistic) Reset() {
	s.TurnStartTs = 0
	s.AsrFirstTextTs = 0
	s.AsrFinalTextTs = 0
	s.LlmStartTs = 0
	s.LlmFirstTokenTs = 0
	s.LlmEndTs = 0
	s.TtsStartTs = 0
	s.TtsFirstFrameTs = 0
	s.TtsStopTs = 0
}

func (state *ClientState) MarkTurnStart() {
	state.Statistic.Reset()
	state.Statistic.TurnStartTs = time.Now().UnixMilli()
}

func (state *ClientState) MarkAsrFirstText() {
	if state.Statistic.AsrFirstTextTs == 0 {
		state.Statistic.AsrFirstTextTs = time.Now().UnixMilli()
	}
}

func (state *ClientState) MarkAsrFinalText() {
	if state.Statistic.AsrFinalTextTs == 0 {
		state.Statistic.AsrFinalTextTs = time.Now().UnixMilli()
	}
}

func (state *ClientState) MarkLlmStart() {
	state.Statistic.LlmStartTs = time.Now().UnixMilli()
}

func (state *ClientState) MarkLlmFirstToken() {
	if state.Statistic.LlmFirstTokenTs == 0 {
		state.Statistic.LlmFirstTokenTs = time.Now().UnixMilli()
	}
}

func (state *ClientState) MarkLlmEnd() {
	state.Statistic.LlmEndTs = time.Now().UnixMilli()
}

func (state *ClientState) MarkTtsStart() {
	state.Statistic.TtsStartTs = time.Now().UnixMilli()
	state.Statistic.TtsFirstFrameTs = 0
	state.Statistic.TtsStopTs = 0
}

func (state *ClientState) MarkTtsFirstFrame() {
	if state.Statistic.TtsFirstFrameTs == 0 {
		state.Statistic.TtsFirstFrameTs = time.Now().UnixMilli()
	}
}

func (state *ClientState) MarkTtsStop() {
	state.Statistic.TtsStopTs = time.Now().UnixMilli()
}

// 兼容旧接口
func (state *ClientState) SetStartAsrTs() { state.MarkTurnStart() }
func (state *ClientState) SetStartLlmTs() { state.MarkLlmStart() }
func (state *ClientState) SetStartTtsTs() { state.MarkTtsStart() }

func (state *ClientState) GetAsrDuration() int64 {
	if state.Statistic.TurnStartTs == 0 {
		return 0
	}
	end := state.Statistic.AsrFinalTextTs
	if end == 0 {
		end = time.Now().UnixMilli()
	}
	return end - state.Statistic.TurnStartTs
}

func (state *ClientState) GetAsrLlmTtsDuration() int64 {
	if state.Statistic.TurnStartTs == 0 {
		return 0
	}
	end := state.Statistic.TtsFirstFrameTs
	if end == 0 {
		end = time.Now().UnixMilli()
	}
	return end - state.Statistic.TurnStartTs
}

func (state *ClientState) GetLlmDuration() int64 {
	if state.Statistic.LlmStartTs == 0 {
		return 0
	}
	end := state.Statistic.LlmEndTs
	if end == 0 {
		end = time.Now().UnixMilli()
	}
	return end - state.Statistic.LlmStartTs
}

func (state *ClientState) GetTtsDuration() int64 {
	if state.Statistic.TtsStartTs == 0 {
		return 0
	}
	end := state.Statistic.TtsStopTs
	if end == 0 {
		end = time.Now().UnixMilli()
	}
	return end - state.Statistic.TtsStartTs
}
