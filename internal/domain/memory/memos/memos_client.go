package memos

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/cloudwego/eino/schema"

	log "xiaozhi-esp32-server-golang/logger"
)

const (
	defaultBaseURL   = "http://localhost:8000"
	defaultTimeoutMS = 10000
)

// Client 是 MemOS 独立 provider 客户端实现。
type Client struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	enableSearch bool
	searchTopK   int

	addMessagePath  string
	getMessagesPath string
	searchPath      string
	flushPath       string
	resetPath       string
}

// GetWithConfig 使用配置初始化 MemOS 客户端。
//
// 基于官方文档目录 /cn/api_docs/core/*，默认 endpoint 使用 core 语义路径，
// 如后续文档字段有差异，可通过 endpoint_* 配置覆盖。
func GetWithConfig(config map[string]interface{}) (*Client, error) {
	if config == nil {
		config = map[string]interface{}{}
	}

	baseURL := getString(config, "base_url", defaultBaseURL)
	apiKey := getString(config, "api_key", "")
	timeoutMS := getInt(config, "timeout_ms", defaultTimeoutMS)
	enableSearch := getBool(config, "enable_search", true)
	searchTopK := getInt(config, "search_top_k", 3)

	if strings.TrimSpace(baseURL) == "" {
		return nil, fmt.Errorf("memos.base_url 配置缺失或为空")
	}
	if searchTopK <= 0 {
		searchTopK = 3
	}
	if timeoutMS <= 0 {
		timeoutMS = defaultTimeoutMS
	}

	client := &Client{
		baseURL:         strings.TrimRight(baseURL, "/"),
		apiKey:          strings.TrimSpace(apiKey),
		httpClient:      &http.Client{Timeout: time.Duration(timeoutMS) * time.Millisecond},
		enableSearch:    enableSearch,
		searchTopK:      searchTopK,
		addMessagePath:  getString(config, "endpoint_add_message", "/api/v1/core/add_message"),
		getMessagesPath: getString(config, "endpoint_get_messages", "/api/v1/core/get_messages"),
		searchPath:      getString(config, "endpoint_search", "/api/v1/core/search"),
		flushPath:       getString(config, "endpoint_flush", "/api/v1/core/flush"),
		resetPath:       getString(config, "endpoint_reset_memory", "/api/v1/core/reset_memory"),
	}

	log.Log().Infof("MemOS 客户端初始化成功, base_url: %s", client.baseURL)
	return client, nil
}

func (c *Client) AddMessage(ctx context.Context, agentID string, msg schema.Message) error {
	payload := map[string]interface{}{
		"agent_id": agentID,
		"messages": []map[string]string{{
			"role":    string(msg.Role),
			"content": msg.Content,
		}},
	}
	_, err := c.requestJSON(ctx, http.MethodPost, c.addMessagePath, payload)
	if err != nil {
		return fmt.Errorf("memos add_message failed: %w", err)
	}
	return nil
}

func (c *Client) GetMessages(ctx context.Context, agentID string, count int) ([]*schema.Message, error) {
	if count <= 0 {
		count = 20
	}
	payload := map[string]interface{}{
		"agent_id": agentID,
		"limit":    count,
	}
	data, err := c.requestJSON(ctx, http.MethodPost, c.getMessagesPath, payload)
	if err != nil {
		return nil, fmt.Errorf("memos get_messages failed: %w", err)
	}

	msgsRaw, ok := data["messages"].([]interface{})
	if !ok {
		return []*schema.Message{}, nil
	}
	messages := make([]*schema.Message, 0, len(msgsRaw))
	for _, item := range msgsRaw {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		role := schema.Assistant
		if r, ok := obj["role"].(string); ok {
			switch strings.ToLower(r) {
			case "user":
				role = schema.User
			case "assistant":
				role = schema.Assistant
			case "system":
				role = schema.System
			}
		}
		content, _ := obj["content"].(string)
		messages = append(messages, &schema.Message{Role: role, Content: content})
	}
	return messages, nil
}

func (c *Client) GetContext(ctx context.Context, agentID string, maxToken int) (string, error) {
	if !c.enableSearch {
		return "", nil
	}
	return c.Search(ctx, agentID, "", c.searchTopK, 0)
}

func (c *Client) Search(ctx context.Context, agentID string, query string, topK int, timeRangeDays int64) (string, error) {
	if !c.enableSearch {
		return "", nil
	}
	if topK <= 0 {
		topK = c.searchTopK
	}
	payload := map[string]interface{}{
		"agent_id":        agentID,
		"query":           query,
		"top_k":           topK,
		"time_range_days": timeRangeDays,
	}
	data, err := c.requestJSON(ctx, http.MethodPost, c.searchPath, payload)
	if err != nil {
		return "", fmt.Errorf("memos search failed: %w", err)
	}
	items, ok := data["results"].([]interface{})
	if !ok || len(items) == 0 {
		return "", nil
	}
	lines := make([]string, 0, len(items))
	for _, item := range items {
		obj, ok := item.(map[string]interface{})
		if !ok {
			continue
		}
		text, _ := obj["content"].(string)
		if text == "" {
			text, _ = obj["memory"].(string)
		}
		if text != "" {
			lines = append(lines, "- "+text)
		}
	}
	return strings.Join(lines, "\n"), nil
}

func (c *Client) Flush(ctx context.Context, agentID string) error {
	payload := map[string]interface{}{"agent_id": agentID}
	_, err := c.requestJSON(ctx, http.MethodPost, c.flushPath, payload)
	if err != nil {
		return fmt.Errorf("memos flush failed: %w", err)
	}
	return nil
}

func (c *Client) ResetMemory(ctx context.Context, agentID string) error {
	payload := map[string]interface{}{"agent_id": agentID}
	_, err := c.requestJSON(ctx, http.MethodPost, c.resetPath, payload)
	if err != nil {
		return fmt.Errorf("memos reset_memory failed: %w", err)
	}
	return nil
}

func (c *Client) requestJSON(ctx context.Context, method, path string, payload map[string]interface{}) (map[string]interface{}, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status=%d, body=%s", resp.StatusCode, string(respBytes))
	}

	if len(respBytes) == 0 {
		return map[string]interface{}{}, nil
	}

	var out map[string]interface{}
	if err := json.Unmarshal(respBytes, &out); err != nil {
		return nil, fmt.Errorf("invalid json response: %w", err)
	}
	if data, ok := out["data"].(map[string]interface{}); ok {
		return data, nil
	}
	return out, nil
}

func getString(config map[string]interface{}, key, defaultValue string) string {
	if v, ok := config[key]; ok {
		if s, ok := v.(string); ok {
			return s
		}
	}
	return defaultValue
}

func getInt(config map[string]interface{}, key string, defaultValue int) int {
	if v, ok := config[key]; ok {
		switch value := v.(type) {
		case int:
			return value
		case int32:
			return int(value)
		case int64:
			return int(value)
		case float64:
			return int(value)
		}
	}
	return defaultValue
}

func getBool(config map[string]interface{}, key string, defaultValue bool) bool {
	if v, ok := config[key]; ok {
		if b, ok := v.(bool); ok {
			return b
		}
	}
	return defaultValue
}
