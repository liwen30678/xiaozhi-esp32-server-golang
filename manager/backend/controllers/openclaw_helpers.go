package controllers

import (
	"encoding/json"
	"strings"

	"xiaozhi/manager/backend/models"
)

type OpenClawConfigResponse struct {
	Allowed       bool     `json:"allowed"`
	EnterKeywords []string `json:"enter_keywords"`
	ExitKeywords  []string `json:"exit_keywords"`
}

func normalizeOpenClawKeywords(keywords []string) []string {
	normalized := make([]string, 0, len(keywords))
	seen := make(map[string]struct{}, len(keywords))
	for _, keyword := range keywords {
		trimmed := strings.TrimSpace(keyword)
		if trimmed == "" {
			continue
		}
		if _, exists := seen[trimmed]; exists {
			continue
		}
		seen[trimmed] = struct{}{}
		normalized = append(normalized, trimmed)
	}
	return normalized
}

func parseOpenClawKeywords(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}

	var parsed []string
	if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
		return normalizeOpenClawKeywords(parsed)
	}

	// 兼容逗号分隔旧格式
	parts := strings.Split(raw, ",")
	return normalizeOpenClawKeywords(parts)
}

func mustOpenClawKeywordsJSON(keywords []string) string {
	normalized := normalizeOpenClawKeywords(keywords)
	data, err := json.Marshal(normalized)
	if err != nil {
		return "[]"
	}
	return string(data)
}

func normalizeOpenClawKeywordsRaw(raw string) string {
	return mustOpenClawKeywordsJSON(parseOpenClawKeywords(raw))
}

func buildOpenClawConfigFromAgent(agent models.Agent) OpenClawConfigResponse {
	return OpenClawConfigResponse{
		Allowed:       agent.OpenClawEnabled,
		EnterKeywords: parseOpenClawKeywords(agent.OpenClawEnterKeywords),
		ExitKeywords:  parseOpenClawKeywords(agent.OpenClawExitKeywords),
	}
}
