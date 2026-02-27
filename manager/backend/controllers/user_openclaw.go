package controllers

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"xiaozhi/manager/backend/models"
	mcpmarket "xiaozhi/manager/backend/services/mcp_market"

	"github.com/gin-gonic/gin"
)

type userOpenClawConfigView struct {
	ID           uint       `json:"id"`
	Name         string     `json:"name"`
	BaseURL      string     `json:"base_url"`
	AuthType     string     `json:"auth_type"`
	TokenMask    string     `json:"token_mask"`
	Enabled      bool       `json:"enabled"`
	IsDefault    bool       `json:"is_default"`
	HealthStatus string     `json:"health_status"`
	LastHealthAt *time.Time `json:"last_health_at"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}

func toOpenClawConfigView(cfg *models.UserOpenClawConfig) userOpenClawConfigView {
	return userOpenClawConfigView{
		ID:           cfg.ID,
		Name:         cfg.Name,
		BaseURL:      cfg.BaseURL,
		AuthType:     cfg.AuthType,
		TokenMask:    cfg.TokenMask,
		Enabled:      cfg.Enabled,
		IsDefault:    cfg.IsDefault,
		HealthStatus: cfg.HealthStatus,
		LastHealthAt: cfg.LastHealthAt,
		CreatedAt:    cfg.CreatedAt,
		UpdatedAt:    cfg.UpdatedAt,
	}
}

func (uc *UserController) listOwnedOpenClawConfigs(userID uint) ([]models.UserOpenClawConfig, error) {
	var items []models.UserOpenClawConfig
	err := uc.DB.Where("user_id = ?", userID).Order("is_default DESC, id DESC").Find(&items).Error
	return items, err
}

func (uc *UserController) getOwnedOpenClawConfig(userID uint, id uint) (*models.UserOpenClawConfig, error) {
	var item models.UserOpenClawConfig
	if err := uc.DB.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		return nil, err
	}
	return &item, nil
}

func (uc *UserController) assertOpenClawConfigOwnership(userID uint, configID *uint) error {
	if configID == nil {
		return nil
	}
	_, err := uc.getOwnedOpenClawConfig(userID, *configID)
	return err
}

func (uc *UserController) GetOpenClawConfigs(c *gin.Context) {
	userID, _ := c.Get("user_id")
	items, err := uc.listOwnedOpenClawConfigs(userID.(uint))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取 OpenClaw 配置失败"})
		return
	}
	resp := make([]userOpenClawConfigView, 0, len(items))
	for i := range items {
		resp = append(resp, toOpenClawConfigView(&items[i]))
	}
	c.JSON(http.StatusOK, gin.H{"data": resp})
}

func (uc *UserController) CreateOpenClawConfig(c *gin.Context) {
	userID, _ := c.Get("user_id")
	var req struct {
		Name      string `json:"name" binding:"required,min=1,max=100"`
		BaseURL   string `json:"base_url" binding:"required"`
		AuthType  string `json:"auth_type"`
		Token     string `json:"token"`
		Enabled   *bool  `json:"enabled"`
		IsDefault bool   `json:"is_default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	if !validateOpenClawBaseURL(req.BaseURL) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "base_url 非法，仅支持 http/https"})
		return
	}
	authType := normalizeOpenClawAuthType(req.AuthType)
	token := strings.TrimSpace(req.Token)
	if authType == openClawAuthTypeBearer && token == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "bearer 模式 token 不能为空"})
		return
	}
	cipherText, nonce, err := mcpmarket.EncryptText(token)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "token 加密失败: " + err.Error()})
		return
	}
	enabled := true
	if req.Enabled != nil {
		enabled = *req.Enabled
	}
	item := models.UserOpenClawConfig{
		UserID:          userID.(uint),
		Name:            req.Name,
		BaseURL:         strings.TrimSpace(req.BaseURL),
		AuthType:        authType,
		TokenCiphertext: cipherText,
		TokenNonce:      nonce,
		TokenMask:       mcpmarket.MaskToken(token),
		Enabled:         enabled,
		IsDefault:       req.IsDefault,
		HealthStatus:    "unknown",
	}
	if req.IsDefault {
		uc.DB.Model(&models.UserOpenClawConfig{}).Where("user_id = ?", userID).Update("is_default", false)
	}
	if err := uc.DB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建 OpenClaw 配置失败"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": toOpenClawConfigView(&item)})
}

func (uc *UserController) UpdateOpenClawConfig(c *gin.Context) {
	userID, _ := c.Get("user_id")
	id, _ := strconv.Atoi(c.Param("id"))
	item, err := uc.getOwnedOpenClawConfig(userID.(uint), uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "配置不存在"})
		return
	}
	var req struct {
		Name      string `json:"name"`
		BaseURL   string `json:"base_url"`
		AuthType  string `json:"auth_type"`
		Token     string `json:"token"`
		Enabled   *bool  `json:"enabled"`
		IsDefault *bool  `json:"is_default"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	if strings.TrimSpace(req.Name) != "" {
		item.Name = strings.TrimSpace(req.Name)
	}
	if strings.TrimSpace(req.BaseURL) != "" {
		if !validateOpenClawBaseURL(req.BaseURL) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "base_url 非法，仅支持 http/https"})
			return
		}
		item.BaseURL = strings.TrimSpace(req.BaseURL)
	}
	if strings.TrimSpace(req.AuthType) != "" {
		item.AuthType = normalizeOpenClawAuthType(req.AuthType)
	}
	if req.Enabled != nil {
		item.Enabled = *req.Enabled
	}
	if req.IsDefault != nil {
		item.IsDefault = *req.IsDefault
		if item.IsDefault {
			uc.DB.Model(&models.UserOpenClawConfig{}).Where("user_id = ? AND id <> ?", userID, item.ID).Update("is_default", false)
		}
	}
	if strings.TrimSpace(req.Token) != "" {
		cipherText, nonce, err := mcpmarket.EncryptText(strings.TrimSpace(req.Token))
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": "token 加密失败: " + err.Error()})
			return
		}
		item.TokenCiphertext = cipherText
		item.TokenNonce = nonce
		item.TokenMask = mcpmarket.MaskToken(req.Token)
	}
	if err := uc.DB.Save(item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新 OpenClaw 配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": toOpenClawConfigView(item)})
}

func (uc *UserController) DeleteOpenClawConfig(c *gin.Context) {
	userID, _ := c.Get("user_id")
	id, _ := strconv.Atoi(c.Param("id"))
	item, err := uc.getOwnedOpenClawConfig(userID.(uint), uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "配置不存在"})
		return
	}
	if err := uc.DB.Model(&models.Agent{}).
		Where("user_id = ? AND openclaw_config_id = ?", userID, item.ID).
		Updates(map[string]interface{}{"openclaw_config_id": nil, "openclaw_enabled": false}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "解绑智能体 OpenClaw 失败"})
		return
	}
	if err := uc.DB.Delete(item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除 OpenClaw 配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true})
}

func (uc *UserController) TestOpenClawConfig(c *gin.Context) {
	userID, _ := c.Get("user_id")
	id, _ := strconv.Atoi(c.Param("id"))
	item, err := uc.getOwnedOpenClawConfig(userID.(uint), uint(id))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "配置不存在"})
		return
	}
	now := time.Now()
	item.LastHealthAt = &now
	if !item.Enabled {
		item.HealthStatus = "disabled"
	} else {
		item.HealthStatus = "ok"
	}
	if err := uc.DB.Save(item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "测试状态更新失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"status": item.HealthStatus, "tested_at": item.LastHealthAt}})
}

func (uc *UserController) UpdateAgentOpenClawBinding(c *gin.Context) {
	userID, _ := c.Get("user_id")
	id := c.Param("id")
	var agent models.Agent
	if err := uc.DB.Where("id = ? AND user_id = ?", id, userID).First(&agent).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "智能体不存在"})
		return
	}
	var req struct {
		OpenClawConfigID *uint  `json:"openclaw_config_id"`
		OpenClawEnabled  *bool  `json:"openclaw_enabled"`
		EnterKeywords    string `json:"openclaw_enter_keywords"`
		ExitKeywords     string `json:"openclaw_exit_keywords"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	if err := uc.assertOpenClawConfigOwnership(userID.(uint), req.OpenClawConfigID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "openclaw 配置不属于当前用户"})
		return
	}
	agent.OpenClawConfigID = req.OpenClawConfigID
	if req.OpenClawEnabled != nil {
		agent.OpenClawEnabled = *req.OpenClawEnabled
	}
	if strings.TrimSpace(req.EnterKeywords) != "" {
		agent.OpenClawEnterKeywords = normalizeOpenClawKeywordsCSV(req.EnterKeywords)
	}
	if strings.TrimSpace(req.ExitKeywords) != "" {
		agent.OpenClawExitKeywords = normalizeOpenClawKeywordsCSV(req.ExitKeywords)
	}
	if err := uc.DB.Save(&agent).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新智能体 OpenClaw 绑定失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": agent})
}
