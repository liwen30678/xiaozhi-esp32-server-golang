package companion

import (
	"encoding/json"
	"net/http"

	log "xiaozhi-esp32-server-golang/logger"
)

// Handler HTTP处理器
type Handler struct {
	companion *Companion
	getChatManager func(deviceID string) (ChatManager, bool)
}

// ChatManager 定义ChatManager需要的接口
type ChatManager interface {
	SendCustomJson(msg interface{}) error
}

// NewHandler 创建HTTP处理器
func NewHandler(comp *Companion, getChatManager func(deviceID string) (ChatManager, bool)) *Handler {
	return &Handler{
		companion:     comp,
		getChatManager: getChatManager,
	}
}

// handleGenerate 处理生图请求
// POST /admin/companion/generate
// Body: {"device_id":"xx","prompt":"描述文字"}
func (h *Handler) handleGenerate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		DeviceID string `json:"device_id"`
		Prompt   string `json:"prompt"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "请求参数错误: "+err.Error(), http.StatusBadRequest)
		return
	}
	if req.DeviceID == "" {
		http.Error(w, "device_id is required", http.StatusBadRequest)
		return
	}
	if req.Prompt == "" {
		http.Error(w, "prompt is required", http.StatusBadRequest)
		return
	}

	// 获取设备ChatManager
	cm, exists := h.getChatManager(req.DeviceID)
	if !exists || cm == nil {
		http.Error(w, "设备不在线或未找到", http.StatusNotFound)
		return
	}

	// 生成图片
	msg, err := h.companion.GenerateAndPush(r.Context(), req.Prompt)
	if err != nil {
		log.Errorf("[Companion] 生图失败: %v", err)
		http.Error(w, "生图失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	// 推送到ESP
	if err := cm.SendCustomJson(msg); err != nil {
		log.Errorf("[Companion] 推送图片消息失败: %v", err)
		http.Error(w, "推送失败: "+err.Error(), http.StatusInternalServerError)
		return
	}

	log.Infof("[Companion] 已推送图片到设备 %s", req.DeviceID)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":   true,
		"message":   "图片已推送",
		"image_url": msg.ImageURL,
	})
}

// RegisterRoutes 注册HTTP路由
func (h *Handler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("/admin/companion/generate", h.handleGenerate)
}
