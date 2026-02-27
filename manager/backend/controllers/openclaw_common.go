package controllers

import (
	"net/url"
	"strings"
)

const (
	openClawAuthTypeBearer = "bearer"
	openClawAuthTypeNone   = "none"
)

func splitOpenClawKeywords(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return []string{}
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		v := strings.ToLower(strings.TrimSpace(part))
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func normalizeOpenClawKeywordsCSV(raw string) string {
	return strings.Join(splitOpenClawKeywords(raw), ",")
}

func normalizeOpenClawAuthType(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case openClawAuthTypeNone:
		return openClawAuthTypeNone
	default:
		return openClawAuthTypeBearer
	}
}

func validateOpenClawBaseURL(raw string) bool {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u == nil {
		return false
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return false
	}
	return strings.TrimSpace(u.Host) != ""
}
