package controllers

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"xiaozhi/manager/backend/models"
	mcpmarket "xiaozhi/manager/backend/services/mcp_market"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func (ac *AdminController) GetOpenClawRuntimeConfig(c *gin.Context) {
	deviceID := strings.TrimSpace(c.Query("device_id"))
	configIDStr := strings.TrimSpace(c.Query("config_id"))
	if deviceID == "" || configIDStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id and config_id are required"})
		return
	}
	configID, err := strconv.Atoi(configIDStr)
	if err != nil || configID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid config_id"})
		return
	}

	var device models.Device
	if err := ac.DB.Where("device_name = ?", deviceID).First(&device).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "device not found"})
		return
	}

	var cfg models.UserOpenClawConfig
	if err := ac.DB.Where("id = ? AND user_id = ? AND enabled = ?", uint(configID), device.UserID, true).First(&cfg).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "openclaw config not found"})
		return
	}

	token, err := mcpmarket.DecryptText(cfg.TokenCiphertext, cfg.TokenNonce)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "decrypt token failed"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"base_url":  cfg.BaseURL,
		"auth_type": cfg.AuthType,
		"token":     token,
	}})
}

func (ac *AdminController) CreateOpenClawOfflineMessage(c *gin.Context) {
	var req struct {
		DeviceID         string `json:"device_id" binding:"required"`
		UserID           uint   `json:"user_id"`
		AgentID          uint   `json:"agent_id"`
		OpenClawConfigID *uint  `json:"openclaw_config_id"`
		TaskID           string `json:"task_id"`
		MessageType      string `json:"message_type"`
		PayloadJSON      string `json:"payload_json" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if strings.TrimSpace(req.MessageType) == "" {
		req.MessageType = "text"
	}
	item := models.OpenClawOfflineMessage{
		DeviceID:         strings.TrimSpace(req.DeviceID),
		UserID:           req.UserID,
		AgentID:          req.AgentID,
		OpenClawConfigID: req.OpenClawConfigID,
		TaskID:           strings.TrimSpace(req.TaskID),
		MessageType:      req.MessageType,
		PayloadJSON:      req.PayloadJSON,
		Status:           "pending",
	}
	if item.DeviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id is required"})
		return
	}
	if item.TaskID == "" {
		item.TaskID = fmt.Sprintf("offline-%s-%d", item.DeviceID, time.Now().UnixNano())
	}

	err := ac.DB.Transaction(func(tx *gorm.DB) error {
		if item.TaskID != "" {
			var existed models.OpenClawOfflineMessage
			if err := tx.Where("task_id = ?", item.TaskID).First(&existed).Error; err == nil {
				item = existed
				return nil
			}
		}
		if err := tx.Create(&item).Error; err != nil {
			return err
		}
		var ids []uint
		if err := tx.Model(&models.OpenClawOfflineMessage{}).
			Where("device_id = ? AND status = ?", item.DeviceID, "pending").
			Order("created_at DESC").
			Offset(100).
			Pluck("id", &ids).Error; err != nil {
			return err
		}
		if len(ids) > 0 {
			if err := tx.Where("id IN ?", ids).Delete(&models.OpenClawOfflineMessage{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "create offline message failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (ac *AdminController) ListOpenClawOfflineMessages(c *gin.Context) {
	deviceID := strings.TrimSpace(c.Query("device_id"))
	status := strings.TrimSpace(c.DefaultQuery("status", "pending"))
	if deviceID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "device_id is required"})
		return
	}
	var items []models.OpenClawOfflineMessage
	if err := ac.DB.Where("device_id = ? AND status = ?", deviceID, status).Order("created_at ASC").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "query offline messages failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (ac *AdminController) MarkOpenClawOfflineMessageDelivered(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid id"})
		return
	}
	now := time.Now()
	if err := ac.DB.Model(&models.OpenClawOfflineMessage{}).Where("id = ?", uint(id)).Updates(map[string]interface{}{
		"status":       "delivered",
		"delivered_at": &now,
	}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "mark delivered failed"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}
