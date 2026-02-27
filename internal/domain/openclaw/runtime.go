package openclaw

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"xiaozhi-esp32-server-golang/internal/util"
)

type RuntimeConfig struct {
	BaseURL  string `json:"base_url"`
	AuthType string `json:"auth_type"`
	Token    string `json:"token"`
}

type OfflineMessage struct {
	ID          uint   `json:"id"`
	DeviceID    string `json:"device_id"`
	PayloadJSON string `json:"payload_json"`
	TaskID      string `json:"task_id"`
}

type GatewayResponse struct {
	Reply   string `json:"reply"`
	TaskID  string `json:"task_id"`
	Pending bool   `json:"pending"`
}

type sessionState struct {
	cfg         RuntimeConfig
	failCount   int
	lastActive  time.Time
	initialized bool
}

type sessionManager struct {
	mu       sync.Mutex
	sessions map[string]*sessionState
}

var mgr = &sessionManager{sessions: map[string]*sessionState{}}

func sessionKey(deviceID, cfgID string) string {
	return strings.TrimSpace(deviceID) + "||" + strings.TrimSpace(cfgID)
}

func (m *sessionManager) getOrInitConfig(ctx context.Context, deviceID, configID string) (*sessionState, error) {
	key := sessionKey(deviceID, configID)
	m.mu.Lock()
	st, ok := m.sessions[key]
	if !ok {
		st = &sessionState{}
		m.sessions[key] = st
	}
	if st.initialized {
		m.mu.Unlock()
		return st, nil
	}
	m.mu.Unlock()

	cfg, err := fetchRuntimeConfig(ctx, deviceID, configID)
	if err != nil {
		return nil, err
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if !st.initialized {
		st.cfg = cfg
		st.initialized = true
	}
	st.lastActive = time.Now()
	return st, nil
}

func HandleRequest(ctx context.Context, deviceID, agentID string, userID uint, configID string, text string) (string, error) {
	st, err := mgr.getOrInitConfig(ctx, deviceID, configID)
	if err != nil {
		return "", err
	}
	reply, err := sendGatewayRequest(ctx, st.cfg, text, deviceID)
	if err == nil {
		mgr.mu.Lock()
		st.failCount = 0
		st.lastActive = time.Now()
		mgr.mu.Unlock()
		if strings.TrimSpace(reply.Reply) != "" {
			return reply.Reply, nil
		}
		if reply.Pending {
			return "OpenClaw 任务处理中，完成后会推送结果。", nil
		}
		return "OpenClaw 已接收请求。", nil
	}

	mgr.mu.Lock()
	st.failCount++
	fc := st.failCount
	mgr.mu.Unlock()

	go func() {
		// 异步兜底：长超时再请求一次，成功则落离线池
		bg, cancel := context.WithTimeout(context.Background(), 60*time.Second)
		defer cancel()
		resp, e := sendGatewayRequest(bg, st.cfg, text, deviceID)
		if e != nil {
			return
		}
		payload := strings.TrimSpace(resp.Reply)
		if payload == "" {
			payload = "OpenClaw 任务完成。"
		}
		_ = createOfflineMessage(bg, deviceID, userID, agentID, configID, resp.TaskID, payload)
	}()

	if fc >= 3 {
		return "OpenClaw 暂不可用，已自动切回默认助手。", ErrFallbackToLLM
	}
	return "OpenClaw 处理中，请稍后。", nil
}

var ErrFallbackToLLM = fmt.Errorf("openclaw fallback to llm")

func sendGatewayRequest(ctx context.Context, cfg RuntimeConfig, text, deviceID string) (GatewayResponse, error) {
	var out GatewayResponse
	body := map[string]interface{}{"message": text, "device_id": deviceID}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, cfg.BaseURL, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	if strings.ToLower(cfg.AuthType) == "bearer" && strings.TrimSpace(cfg.Token) != "" {
		req.Header.Set("Authorization", "Bearer "+cfg.Token)
	}
	cli := &http.Client{Timeout: 8 * time.Second}
	resp, err := cli.Do(req)
	if err != nil {
		return out, err
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return out, fmt.Errorf("gateway http %d", resp.StatusCode)
	}
	_ = json.Unmarshal(b, &out)
	if out.Reply == "" && !out.Pending {
		out.Reply = strings.TrimSpace(string(b))
	}
	return out, nil
}

func fetchRuntimeConfig(ctx context.Context, deviceID, configID string) (RuntimeConfig, error) {
	q := url.Values{}
	q.Set("device_id", strings.TrimSpace(deviceID))
	q.Set("config_id", strings.TrimSpace(configID))
	runtimeURL := strings.TrimSuffix(util.GetBackendURL(), "/") + "/api/internal/openclaw/runtime-config?" + q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, runtimeURL, nil)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return RuntimeConfig{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return RuntimeConfig{}, fmt.Errorf("runtime config http %d", resp.StatusCode)
	}
	var result struct {
		Data RuntimeConfig `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return RuntimeConfig{}, err
	}
	return result.Data, nil
}

func createOfflineMessage(ctx context.Context, deviceID string, userID uint, agentID, configID, taskID, payload string) error {
	url := strings.TrimSuffix(util.GetBackendURL(), "/") + "/api/internal/openclaw/offline-messages"
	cfgUint, _ := strconv.Atoi(strings.TrimSpace(configID))
	agentUint, _ := strconv.Atoi(strings.TrimSpace(agentID))
	body := map[string]interface{}{
		"device_id":          deviceID,
		"user_id":            userID,
		"agent_id":           agentUint,
		"openclaw_config_id": uint(cfgUint),
		"task_id":            taskID,
		"message_type":       "text",
		"payload_json":       payload,
	}
	raw, _ := json.Marshal(body)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("offline create http %d", resp.StatusCode)
	}
	return nil
}

func ListPendingOfflineMessages(ctx context.Context, deviceID string) ([]OfflineMessage, error) {
	q := url.Values{}
	q.Set("device_id", strings.TrimSpace(deviceID))
	q.Set("status", "pending")
	listURL := strings.TrimSuffix(util.GetBackendURL(), "/") + "/api/internal/openclaw/offline-messages?" + q.Encode()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, listURL, nil)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("offline list http %d", resp.StatusCode)
	}
	var result struct {
		Data []OfflineMessage `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}
	return result.Data, nil
}

func MarkOfflineMessageDelivered(ctx context.Context, id uint) error {
	url := fmt.Sprintf("%s/api/internal/openclaw/offline-messages/%d/delivered", strings.TrimSuffix(util.GetBackendURL(), "/"), id)
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, url, nil)
	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return fmt.Errorf("offline mark delivered http %d", resp.StatusCode)
	}
	return nil
}
