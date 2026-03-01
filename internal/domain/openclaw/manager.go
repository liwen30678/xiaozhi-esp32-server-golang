package openclaw

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"xiaozhi-esp32-server-golang/logger"

	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	cmap "github.com/orcaman/concurrent-map/v2"
)

const (
	MaxOfflineMessagesPerDevice = 20
	OfflineMessageTTL           = 24 * time.Hour
)

type WSMessage struct {
	ID            string          `json:"id"`
	Timestamp     int64           `json:"timestamp"`
	Type          string          `json:"type"`
	CorrelationID string          `json:"correlation_id,omitempty"`
	Payload       json.RawMessage `json:"payload"`
}

type MessagePayload struct {
	Content   string                 `json:"content"`
	SessionID string                 `json:"session_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type ResponsePayload struct {
	Content   string                 `json:"content"`
	SessionID string                 `json:"session_id,omitempty"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

type OfflineMessage struct {
	Text          string
	CorrelationID string
	CreatedAt     time.Time
}

type pendingRoute struct {
	DeviceID  string
	CreatedAt time.Time
}

type AgentSession struct {
	agentID string
	conn    *websocket.Conn

	ctx    context.Context
	cancel context.CancelFunc

	writeMu sync.Mutex
	pending sync.Map // correlation_id -> pendingRoute
}

func newAgentSession(agentID string, conn *websocket.Conn) *AgentSession {
	ctx, cancel := context.WithCancel(context.Background())
	return &AgentSession{
		agentID: agentID,
		conn:    conn,
		ctx:     ctx,
		cancel:  cancel,
	}
}

func (s *AgentSession) Send(msg WSMessage) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return err
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	return s.conn.WriteMessage(websocket.TextMessage, data)
}

func (s *AgentSession) TrackPending(correlationID string, deviceID string) {
	if strings.TrimSpace(correlationID) == "" || strings.TrimSpace(deviceID) == "" {
		return
	}
	s.pending.Store(correlationID, pendingRoute{
		DeviceID:  deviceID,
		CreatedAt: time.Now(),
	})
}

func (s *AgentSession) RemovePending(correlationID string) {
	if strings.TrimSpace(correlationID) == "" {
		return
	}
	s.pending.Delete(correlationID)
}

func (s *AgentSession) ResolvePending(correlationID string) (string, bool) {
	if strings.TrimSpace(correlationID) == "" {
		return "", false
	}

	value, ok := s.pending.Load(correlationID)
	if !ok {
		return "", false
	}
	s.pending.Delete(correlationID)

	route, ok := value.(pendingRoute)
	if !ok {
		return "", false
	}
	return route.DeviceID, route.DeviceID != ""
}

func (s *AgentSession) IsSameConn(conn *websocket.Conn) bool {
	return s.conn == conn
}

func (s *AgentSession) Close() {
	s.cancel()
	_ = s.conn.Close()
}

type Manager struct {
	sessions cmap.ConcurrentMap[string, *AgentSession]

	offlineMu sync.Mutex
	offline   map[string][]OfflineMessage
}

var (
	defaultManager *Manager
	managerOnce    sync.Once
)

func GetManager() *Manager {
	managerOnce.Do(func() {
		defaultManager = &Manager{
			sessions: cmap.New[*AgentSession](),
			offline:  make(map[string][]OfflineMessage),
		}
	})
	return defaultManager
}

func (m *Manager) RegisterAgentConnection(agentID string, conn *websocket.Conn) *AgentSession {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}

	newSession := newAgentSession(agentID, conn)
	if oldSession, ok := m.sessions.Get(agentID); ok && oldSession != nil {
		oldSession.Close()
	}
	m.sessions.Set(agentID, newSession)
	return newSession
}

func (m *Manager) UnregisterAgentConnection(agentID string, session *AgentSession) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return
	}

	current, ok := m.sessions.Get(agentID)
	if !ok || current == nil {
		return
	}

	if session == nil || current == session {
		m.sessions.Remove(agentID)
	}
}

func (m *Manager) GetAgentSession(agentID string) *AgentSession {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return nil
	}
	session, ok := m.sessions.Get(agentID)
	if !ok {
		return nil
	}
	return session
}

func (m *Manager) SendMessage(agentID string, deviceID string, content string, sessionID string) (string, error) {
	agentID = strings.TrimSpace(agentID)
	deviceID = strings.TrimSpace(deviceID)
	content = strings.TrimSpace(content)

	if agentID == "" {
		return "", fmt.Errorf("agentID is required")
	}
	if deviceID == "" {
		return "", fmt.Errorf("deviceID is required")
	}
	if content == "" {
		return "", fmt.Errorf("content is required")
	}

	session := m.GetAgentSession(agentID)
	if session == nil {
		return "", fmt.Errorf("openclaw session not found for agent %s", agentID)
	}

	messageID := uuid.NewString()
	payloadBytes, err := json.Marshal(MessagePayload{
		Content:   content,
		SessionID: strings.TrimSpace(sessionID),
		Metadata: map[string]interface{}{
			"device_id": deviceID,
			"agent_id":  agentID,
		},
	})
	if err != nil {
		return "", err
	}

	session.TrackPending(messageID, deviceID)
	err = session.Send(WSMessage{
		ID:        messageID,
		Timestamp: time.Now().UnixMilli(),
		Type:      "message",
		Payload:   payloadBytes,
	})
	if err != nil {
		session.RemovePending(messageID)
		return "", err
	}

	return messageID, nil
}

func (m *Manager) HandleResponse(
	agentID string,
	session *AgentSession,
	correlationID string,
	payload ResponsePayload,
	deliver func(deviceID string, text string) bool,
) {
	content := strings.TrimSpace(payload.Content)
	if content == "" {
		return
	}

	deviceID := ""
	if session != nil {
		if resolvedDeviceID, ok := session.ResolvePending(correlationID); ok {
			deviceID = strings.TrimSpace(resolvedDeviceID)
		}
	}

	if deviceID == "" && payload.Metadata != nil {
		if rawDeviceID, ok := payload.Metadata["device_id"].(string); ok {
			deviceID = strings.TrimSpace(rawDeviceID)
		}
	}

	if deviceID == "" {
		logger.Warnf("OpenClaw response missing device route, agent=%s correlation_id=%s", agentID, correlationID)
		return
	}

	if deliver != nil && deliver(deviceID, content) {
		return
	}

	m.AddOfflineMessage(deviceID, content, correlationID)
}

func (m *Manager) AddOfflineMessage(deviceID string, text string, correlationID string) {
	deviceID = strings.TrimSpace(deviceID)
	text = strings.TrimSpace(text)
	if deviceID == "" || text == "" {
		return
	}

	m.offlineMu.Lock()
	defer m.offlineMu.Unlock()

	m.pruneOfflineLocked(deviceID)
	msgList := append(m.offline[deviceID], OfflineMessage{
		Text:          text,
		CorrelationID: correlationID,
		CreatedAt:     time.Now(),
	})
	if len(msgList) > MaxOfflineMessagesPerDevice {
		msgList = msgList[len(msgList)-MaxOfflineMessagesPerDevice:]
	}
	m.offline[deviceID] = msgList
}

func (m *Manager) ReplayOfflineMessages(deviceID string, deliver func(msg OfflineMessage) error) (int, int) {
	deviceID = strings.TrimSpace(deviceID)
	if deviceID == "" || deliver == nil {
		return 0, 0
	}

	m.offlineMu.Lock()
	m.pruneOfflineLocked(deviceID)
	snapshot := append([]OfflineMessage(nil), m.offline[deviceID]...)
	m.offlineMu.Unlock()

	delivered := 0
	for _, msg := range snapshot {
		if err := deliver(msg); err != nil {
			break
		}
		delivered++
	}

	m.offlineMu.Lock()
	defer m.offlineMu.Unlock()

	m.pruneOfflineLocked(deviceID)
	current := m.offline[deviceID]
	if delivered > 0 {
		if delivered >= len(current) {
			delete(m.offline, deviceID)
			return delivered, 0
		}
		m.offline[deviceID] = current[delivered:]
		current = m.offline[deviceID]
	}
	return delivered, len(current)
}

func (m *Manager) pruneOfflineLocked(deviceID string) {
	msgList, exists := m.offline[deviceID]
	if !exists || len(msgList) == 0 {
		delete(m.offline, deviceID)
		return
	}

	now := time.Now()
	filtered := make([]OfflineMessage, 0, len(msgList))
	for _, msg := range msgList {
		if msg.CreatedAt.IsZero() {
			continue
		}
		if now.Sub(msg.CreatedAt) > OfflineMessageTTL {
			continue
		}
		filtered = append(filtered, msg)
	}

	if len(filtered) == 0 {
		delete(m.offline, deviceID)
		return
	}
	m.offline[deviceID] = filtered
}
