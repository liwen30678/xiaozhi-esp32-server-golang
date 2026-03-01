package websocket

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"
	"xiaozhi-esp32-server-golang/internal/domain/openclaw"
	log "xiaozhi-esp32-server-golang/logger"

	"github.com/golang-jwt/jwt/v4"
	"github.com/google/uuid"
	gws "github.com/gorilla/websocket"
)

type OpenClawClaims struct {
	UserID     uint   `json:"userId"`
	AgentID    string `json:"agentId"`
	EndpointID string `json:"endpointId"`
	Purpose    string `json:"purpose"`
	jwt.RegisteredClaims
}

func (s *WebSocketServer) handleOpenClawWebSocket(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	if token == "" {
		http.Error(w, "missing token", http.StatusUnauthorized)
		return
	}

	claims, err := s.parseOpenClawToken(token)
	if err != nil {
		log.Warnf("OpenClaw token parse failed: %v", err)
		http.Error(w, "invalid token", http.StatusUnauthorized)
		return
	}

	agentID := strings.TrimSpace(claims.AgentID)
	if agentID == "" {
		http.Error(w, "invalid token: missing agent_id", http.StatusUnauthorized)
		return
	}
	if claims.Purpose != "" && claims.Purpose != "openclaw-endpoint" {
		http.Error(w, "invalid token purpose", http.StatusUnauthorized)
		return
	}

	conn, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Errorf("OpenClaw websocket upgrade failed: %v", err)
		return
	}

	manager := openclaw.GetManager()
	session := manager.RegisterAgentConnection(agentID, conn)
	if session == nil {
		_ = conn.Close()
		log.Errorf("failed to init openclaw session, agent=%s", agentID)
		return
	}
	defer manager.UnregisterAgentConnection(agentID, session)

	if err := sendOpenClawHandshakeAck(session); err != nil {
		log.Errorf("Send OpenClaw handshake_ack failed, agent=%s err=%v", agentID, err)
		return
	}
	log.Infof("OpenClaw connected, agent=%s endpoint=%s", agentID, claims.EndpointID)

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			log.Infof("OpenClaw websocket closed, agent=%s err=%v", agentID, err)
			return
		}
		if msgType != gws.TextMessage {
			continue
		}

		var wsMsg openclaw.WSMessage
		if err := json.Unmarshal(data, &wsMsg); err != nil {
			log.Warnf("OpenClaw message decode failed, agent=%s err=%v", agentID, err)
			continue
		}

		switch wsMsg.Type {
		case "handshake":
			log.Infof("OpenClaw handshake received, agent=%s", agentID)
		case "ping":
			if err := replyOpenClawPong(session, wsMsg); err != nil {
				log.Warnf("Reply OpenClaw pong failed, agent=%s err=%v", agentID, err)
			}
		case "response":
			handleOpenClawResponse(agentID, session, wsMsg, s.onOpenClawResponse)
		case "error":
			log.Warnf("OpenClaw returned error, agent=%s payload=%s", agentID, string(wsMsg.Payload))
		case "close":
			log.Infof("OpenClaw requested close, agent=%s", agentID)
			return
		default:
			log.Warnf("OpenClaw unknown message type, agent=%s type=%s", agentID, wsMsg.Type)
		}
	}
}

func handleOpenClawResponse(agentID string, session *openclaw.AgentSession, wsMsg openclaw.WSMessage, deliver func(deviceID string, text string) bool) {
	var payload openclaw.ResponsePayload
	if err := json.Unmarshal(wsMsg.Payload, &payload); err != nil {
		log.Warnf("OpenClaw response payload decode failed, agent=%s err=%v", agentID, err)
		return
	}
	openclaw.GetManager().HandleResponse(agentID, session, wsMsg.CorrelationID, payload, deliver)
}

func sendOpenClawHandshakeAck(session *openclaw.AgentSession) error {
	payloadBytes, err := json.Marshal(map[string]interface{}{
		"version": "1.0.0",
		"server":  "xiaozhi-esp32-server",
	})
	if err != nil {
		return err
	}

	return session.Send(openclaw.WSMessage{
		ID:        uuid.NewString(),
		Timestamp: time.Now().UnixMilli(),
		Type:      "handshake_ack",
		Payload:   payloadBytes,
	})
}

func replyOpenClawPong(session *openclaw.AgentSession, ping openclaw.WSMessage) error {
	type PingPayload struct {
		Seq int `json:"seq"`
	}

	payload := PingPayload{}
	if len(ping.Payload) > 0 {
		_ = json.Unmarshal(ping.Payload, &payload)
	}

	pongPayload, err := json.Marshal(map[string]interface{}{
		"seq": payload.Seq,
	})
	if err != nil {
		return err
	}

	return session.Send(openclaw.WSMessage{
		ID:            uuid.NewString(),
		Timestamp:     time.Now().UnixMilli(),
		Type:          "pong",
		CorrelationID: ping.ID,
		Payload:       pongPayload,
	})
}

func (s *WebSocketServer) parseOpenClawToken(tokenString string) (*OpenClawClaims, error) {
	if strings.HasPrefix(tokenString, "Bearer ") {
		tokenString = strings.TrimSpace(strings.TrimPrefix(tokenString, "Bearer "))
	}

	jwtSecret := []byte("xiaozhi_admin_secret_key")
	token, err := jwt.ParseWithClaims(tokenString, &OpenClawClaims{}, func(token *jwt.Token) (interface{}, error) {
		return jwtSecret, nil
	})
	if err != nil {
		return nil, err
	}

	claims, ok := token.Claims.(*OpenClawClaims)
	if !ok || !token.Valid {
		return nil, jwt.ErrInvalidKey
	}
	return claims, nil
}
