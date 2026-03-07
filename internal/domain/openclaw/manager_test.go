package openclaw

import (
	"strings"
	"testing"
)

func TestBuildOpenClawPromptedContentWrapsUserMessage(t *testing.T) {
	got := buildOpenClawPromptedContent("  天津后天的天气怎么样？  ")

	if !strings.Contains(got, "你正在以语音助手的角色和用户直接对话。") {
		t.Fatalf("missing voice assistant prompt: %q", got)
	}
	if !strings.Contains(got, "回答要简练、口语化、自然，适合直接语音播报。") {
		t.Fatalf("missing concise speech constraint: %q", got)
	}
	if !strings.Contains(got, "用户消息：\n天津后天的天气怎么样？") {
		t.Fatalf("missing wrapped user message: %q", got)
	}
	if strings.Contains(got, "  天津后天的天气怎么样？  ") {
		t.Fatalf("user message was not trimmed: %q", got)
	}
}

func TestExtractOpenClawSentencesKeepsLeadingClauseTogether(t *testing.T) {
	text := "好的，我先帮你查一下今天上海的天气。然后我再继续处理"

	sentences, remaining := extractOpenClawSentences(text, openClawSentenceMinLen, true)

	if len(sentences) != 1 {
		t.Fatalf("unexpected sentence count: got %d want 1", len(sentences))
	}
	if sentences[0] != "好的，我先帮你查一下今天上海的天气。" {
		t.Fatalf("unexpected first sentence: %q", sentences[0])
	}
	if remaining != "然后我再继续处理" {
		t.Fatalf("unexpected remaining text: %q", remaining)
	}
}

func TestExtractOpenClawSentencesMergesShortClauses(t *testing.T) {
	text := "可以。先这样。然后我继续处理。"

	sentences, remaining := extractOpenClawSentences(text, openClawSentenceMinLen, true)

	if len(sentences) != 3 {
		t.Fatalf("unexpected sentence count: got %d want 3", len(sentences))
	}
	if sentences[0] != "可以。" || sentences[1] != "先这样。" || sentences[2] != "然后我继续处理。" {
		t.Fatalf("unexpected sentence split: %+v", sentences)
	}
	if remaining != "" {
		t.Fatalf("unexpected remaining text: %q", remaining)
	}
}

func TestNormalizeOpenClawSpeechTextStripsMarkdownAndBullets(t *testing.T) {
	raw := "🌤️ **天津后天（3月9日）天气预报**\n\n- **温度**：3°C ~ 12°C\n- **天气**：晴朗☀️"

	got := normalizeOpenClawSpeechText(raw)

	if strings.Contains(got, "**") {
		t.Fatalf("unexpected markdown marker in normalized text: %q", got)
	}
	if strings.Contains(got, "\n") {
		t.Fatalf("unexpected newline in normalized text: %q", got)
	}
	if !strings.Contains(got, "温度：3°C ~ 12°C") {
		t.Fatalf("missing normalized temperature segment: %q", got)
	}
	if !strings.Contains(got, "天气：晴朗☀️") {
		t.Fatalf("missing normalized weather segment: %q", got)
	}
}

func TestExtractOpenClawSentencesGroupsWeatherListIntoLongerSegments(t *testing.T) {
	text := "🌤️ **天津后天（3月9日）天气预报**\n\n- **温度**：3°C ~ 12°C\n- **天气**：晴朗☀️\n- **降水**：无降雨\n- **湿度**：15% ~ 38%\n- **风向**：西南风，风速 2-13km/h\n\n后天天津天气不错，晴天为主，最高温度 12°C，最低 3°C。"

	sentences, remaining := extractOpenClawSentences(text, openClawSentenceMinLen, true)

	if len(sentences) == 0 {
		t.Fatal("expected at least one emitted sentence")
	}
	if len(sentences) != 1 {
		t.Fatalf("unexpected sentence count: got %d want 1", len(sentences))
	}
	if remaining != "" {
		t.Fatalf("unexpected remaining text: %q", remaining)
	}
	if strings.Contains(sentences[0], "**") || strings.Contains(sentences[0], "\n") {
		t.Fatalf("unexpected raw markdown in first sentence: %q", sentences[0])
	}
	if !strings.Contains(sentences[0], "温度：") || !strings.Contains(sentences[0], "天气：") {
		t.Fatalf("first sentence still too short: %q", sentences[0])
	}
	if !strings.Contains(sentences[0], "最高温度 12°C") {
		t.Fatalf("missing summary in final sentence: %q", sentences[0])
	}
}
