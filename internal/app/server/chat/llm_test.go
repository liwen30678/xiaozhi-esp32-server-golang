package chat

import (
	"testing"

	mcp_go "github.com/mark3labs/mcp-go/mcp"
)

func TestHandleToolResultAcceptsPlainText(t *testing.T) {
	manager := &LLMManager{}

	result, ok := manager.handleToolResult("普通文本返回")
	if !ok {
		t.Fatal("expected plain text tool result to be accepted")
	}

	if result.IsError {
		t.Fatal("expected plain text tool result not to be marked as error")
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}

	textContent, ok := result.Content[0].(mcp_go.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}

	if textContent.Text != "普通文本返回" {
		t.Fatalf("expected original text to be preserved, got %q", textContent.Text)
	}
}

func TestHandleToolResultAcceptsMCPJSON(t *testing.T) {
	manager := &LLMManager{}

	result, ok := manager.handleToolResult(`{"content":[{"type":"text","text":"json返回"}],"isError":false}`)
	if !ok {
		t.Fatal("expected MCP JSON tool result to be accepted")
	}

	if len(result.Content) != 1 {
		t.Fatalf("expected 1 content item, got %d", len(result.Content))
	}

	textContent, ok := result.Content[0].(mcp_go.TextContent)
	if !ok {
		t.Fatalf("expected text content, got %T", result.Content[0])
	}

	if textContent.Text != "json返回" {
		t.Fatalf("expected parsed text content, got %q", textContent.Text)
	}
}
