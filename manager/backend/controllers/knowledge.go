package controllers

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"xiaozhi/manager/backend/models"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// KnowledgeSearchRequest 主程序调用控制台检索知识库的请求
// 控制台仅进行 provider API 调用与编排，不实现本地检索引擎。
type KnowledgeSearchRequest struct {
	DeviceID string `json:"device_id"`
	AgentID  string `json:"agent_id"`
	Query    string `json:"query" binding:"required"`
	TopK     int    `json:"top_k"`
}

type knowledgeSearchHit struct {
	Content string  `json:"content"`
	Title   string  `json:"title,omitempty"`
	Score   float64 `json:"score,omitempty"`
}

func (ac *AdminController) GetKnowledgeSearchConfigs(c *gin.Context) {
	var configs []models.Config
	if err := ac.DB.Where("type = ?", "knowledge_search").Order("is_default DESC, id DESC").Find(&configs).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取知识库检索配置失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": configs})
}

func (ac *AdminController) CreateKnowledgeSearchConfig(c *gin.Context) {
	var config models.Config
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	config.Type = "knowledge_search"
	ac.createConfigWithType(c, &config)
}

func (ac *AdminController) UpdateKnowledgeSearchConfig(c *gin.Context) {
	ac.updateConfigWithType(c, "knowledge_search")
}

func (ac *AdminController) DeleteKnowledgeSearchConfig(c *gin.Context) {
	ac.deleteConfigWithType(c, "knowledge_search")
}

func (uc *UserController) GetKnowledgeBases(c *gin.Context) {
	userID, _ := c.Get("user_id")
	var items []models.KnowledgeBase
	if err := uc.DB.Where("user_id = ?", userID).Order("id DESC").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取知识库列表失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (uc *UserController) CreateKnowledgeBase(c *gin.Context) {
	userID, _ := c.Get("user_id")
	var req struct {
		Name        string `json:"name" binding:"required,min=1,max=100"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	item := models.KnowledgeBase{UserID: userID.(uint), Name: req.Name, Description: req.Description, Content: req.Content, Status: req.Status}
	if err := uc.DB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建知识库失败"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

func (uc *UserController) GetKnowledgeBase(c *gin.Context) {
	userID, _ := c.Get("user_id")
	id := c.Param("id")
	var item models.KnowledgeBase
	if err := uc.DB.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "知识库不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (uc *UserController) UpdateKnowledgeBase(c *gin.Context) {
	userID, _ := c.Get("user_id")
	id := c.Param("id")
	var item models.KnowledgeBase
	if err := uc.DB.Where("id = ? AND user_id = ?", id, userID).First(&item).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "知识库不存在"})
		return
	}
	var req struct {
		Name        string `json:"name" binding:"required,min=1,max=100"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	item.Name = req.Name
	item.Description = req.Description
	item.Content = req.Content
	if req.Status != "" {
		item.Status = req.Status
	}
	if err := uc.DB.Save(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新知识库失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (uc *UserController) DeleteKnowledgeBase(c *gin.Context) {
	userID, _ := c.Get("user_id")
	id, _ := strconv.Atoi(c.Param("id"))
	if id <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的知识库ID"})
		return
	}
	err := uc.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND user_id = ?", id, userID).Delete(&models.KnowledgeBase{}).Error; err != nil {
			return err
		}
		return tx.Where("knowledge_base_id = ?", id).Delete(&models.AgentKnowledgeBase{}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除知识库失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func (uc *UserController) GetAgentKnowledgeBases(c *gin.Context) {
	userID, _ := c.Get("user_id")
	agentID, _ := strconv.Atoi(c.Param("id"))
	if agentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的智能体ID"})
		return
	}
	if err := uc.assertAgentOwnership(userID.(uint), uint(agentID)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	ids, err := uc.listAgentKnowledgeBaseIDs(uint(agentID))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取智能体知识库关联失败"})
		return
	}
	var items []models.KnowledgeBase
	if len(ids) > 0 {
		if err := uc.DB.Where("id IN ? AND user_id = ?", ids, userID).Find(&items).Error; err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "获取知识库详情失败"})
			return
		}
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"knowledge_base_ids": ids, "knowledge_bases": items}})
}

func (uc *UserController) UpdateAgentKnowledgeBases(c *gin.Context) {
	userID, _ := c.Get("user_id")
	agentID, _ := strconv.Atoi(c.Param("id"))
	if agentID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的智能体ID"})
		return
	}
	if err := uc.assertAgentOwnership(userID.(uint), uint(agentID)); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	var req struct {
		KnowledgeBaseIDs []uint `json:"knowledge_base_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	if err := uc.validateKnowledgeBaseOwnership(userID.(uint), req.KnowledgeBaseIDs); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	err := uc.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("agent_id = ?", agentID).Delete(&models.AgentKnowledgeBase{}).Error; err != nil {
			return err
		}
		for _, kbID := range uniqueUintSlice(req.KnowledgeBaseIDs) {
			item := models.AgentKnowledgeBase{AgentID: uint(agentID), KnowledgeBaseID: kbID}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新智能体知识库关联失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "更新成功", "data": gin.H{"knowledge_base_ids": uniqueUintSlice(req.KnowledgeBaseIDs)}})
}

func (ac *AdminController) InternalKnowledgeSearch(c *gin.Context) {
	var req KnowledgeSearchRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	query := strings.TrimSpace(req.Query)
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query不能为空"})
		return
	}
	if req.TopK <= 0 {
		req.TopK = 5
	}

	agentID := strings.TrimSpace(req.AgentID)
	if agentID == "" && strings.TrimSpace(req.DeviceID) != "" {
		var device models.Device
		if err := ac.DB.Where("device_name = ?", strings.TrimSpace(req.DeviceID)).First(&device).Error; err == nil {
			agentID = fmt.Sprintf("%d", device.AgentID)
		}
	}
	if agentID == "" {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"hits": []knowledgeSearchHit{}, "query": query}})
		return
	}

	var links []models.AgentKnowledgeBase
	if err := ac.DB.Where("agent_id = ?", agentID).Find(&links).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询智能体知识库关联失败"})
		return
	}
	if len(links) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"hits": []knowledgeSearchHit{}, "query": query}})
		return
	}

	kbIDs := make([]uint, 0, len(links))
	for _, item := range links {
		kbIDs = append(kbIDs, item.KnowledgeBaseID)
	}
	var knowledgeBases []models.KnowledgeBase
	if err := ac.DB.Where("id IN ? AND status = ?", kbIDs, "active").Find(&knowledgeBases).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询知识库失败"})
		return
	}
	if len(knowledgeBases) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"hits": []knowledgeSearchHit{}, "query": query}})
		return
	}

	providerConfig, err := ac.getDefaultKnowledgeSearchConfig()
	if err != nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"hits": []knowledgeSearchHit{}, "query": query}, "warning": "知识库检索配置不存在，已降级"})
		return
	}
	hits, callErr := ac.callKnowledgeSearchProvider(providerConfig, req, knowledgeBases)
	if callErr != nil {
		c.JSON(http.StatusOK, gin.H{"data": gin.H{"hits": []knowledgeSearchHit{}, "query": query}, "warning": "知识库检索失败，已降级", "error": callErr.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": gin.H{"hits": hits, "query": query}})
}

func (ac *AdminController) getDefaultKnowledgeSearchConfig() (*models.Config, error) {
	var cfg models.Config
	err := ac.DB.Where("type = ? AND enabled = ?", "knowledge_search", true).Order("is_default DESC, id DESC").First(&cfg).Error
	if err != nil {
		return nil, err
	}
	return &cfg, nil
}

func (ac *AdminController) callKnowledgeSearchProvider(cfg *models.Config, req KnowledgeSearchRequest, knowledgeBases []models.KnowledgeBase) ([]knowledgeSearchHit, error) {
	var providerData map[string]interface{}
	if strings.TrimSpace(cfg.JsonData) != "" {
		if err := json.Unmarshal([]byte(cfg.JsonData), &providerData); err != nil {
			return nil, fmt.Errorf("解析knowledge_search配置失败: %w", err)
		}
	} else {
		providerData = map[string]interface{}{}
	}
	endpoint, _ := providerData["endpoint"].(string)
	endpoint = strings.TrimSpace(endpoint)
	if endpoint == "" {
		return nil, fmt.Errorf("knowledge_search.endpoint 不能为空")
	}
	apiKey, _ := providerData["api_key"].(string)

	payload := map[string]interface{}{
		"provider":        cfg.Provider,
		"query":           strings.TrimSpace(req.Query),
		"top_k":           req.TopK,
		"agent_id":        req.AgentID,
		"device_id":       req.DeviceID,
		"knowledge_bases": knowledgeBases,
	}
	if extras, ok := providerData["extra"]; ok {
		payload["extra"] = extras
	}
	bodyBytes, _ := json.Marshal(payload)

	httpReq, err := http.NewRequest(http.MethodPost, endpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建provider请求失败: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if apiKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+apiKey)
	}
	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("调用provider失败: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("provider返回状态码: %d", resp.StatusCode)
	}
	var result struct {
		Data struct {
			Hits []knowledgeSearchHit `json:"hits"`
		} `json:"data"`
		Hits []knowledgeSearchHit `json:"hits"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("解析provider返回失败: %w", err)
	}
	if len(result.Data.Hits) > 0 {
		return result.Data.Hits, nil
	}
	return result.Hits, nil
}

func (uc *UserController) assertAgentOwnership(userID uint, agentID uint) error {
	var count int64
	if err := uc.DB.Model(&models.Agent{}).Where("id = ? AND user_id = ?", agentID, userID).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("智能体不存在或不属于当前用户")
	}
	return nil
}

func (uc *UserController) validateKnowledgeBaseOwnership(userID uint, knowledgeBaseIDs []uint) error {
	if len(knowledgeBaseIDs) == 0 {
		return nil
	}
	uniqueIDs := uniqueUintSlice(knowledgeBaseIDs)
	var count int64
	if err := uc.DB.Model(&models.KnowledgeBase{}).Where("user_id = ? AND id IN ?", userID, uniqueIDs).Count(&count).Error; err != nil {
		return err
	}
	if count != int64(len(uniqueIDs)) {
		return fmt.Errorf("包含无效或越权的知识库ID")
	}
	return nil
}

func (uc *UserController) listAgentKnowledgeBaseIDs(agentID uint) ([]uint, error) {
	var links []models.AgentKnowledgeBase
	if err := uc.DB.Where("agent_id = ?", agentID).Order("id ASC").Find(&links).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(links))
	for _, link := range links {
		ids = append(ids, link.KnowledgeBaseID)
	}
	return ids, nil
}

func uniqueUintSlice(values []uint) []uint {
	if len(values) == 0 {
		return []uint{}
	}
	m := make(map[uint]struct{}, len(values))
	ret := make([]uint, 0, len(values))
	for _, v := range values {
		if v == 0 {
			continue
		}
		if _, ok := m[v]; ok {
			continue
		}
		m[v] = struct{}{}
		ret = append(ret, v)
	}
	sort.Slice(ret, func(i, j int) bool { return ret[i] < ret[j] })
	return ret
}

func (ac *AdminController) GetUserKnowledgeBasesAdmin(c *gin.Context) {
	userID, _ := strconv.Atoi(c.Param("id"))
	if userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}
	var items []models.KnowledgeBase
	if err := ac.DB.Where("user_id = ?", userID).Order("id DESC").Find(&items).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "获取知识库列表失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": items})
}

func (ac *AdminController) CreateUserKnowledgeBaseAdmin(c *gin.Context) {
	userID, _ := strconv.Atoi(c.Param("id"))
	if userID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的用户ID"})
		return
	}
	var req struct {
		Name        string `json:"name" binding:"required,min=1,max=100"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	if req.Status == "" {
		req.Status = "active"
	}
	item := models.KnowledgeBase{UserID: uint(userID), Name: req.Name, Description: req.Description, Content: req.Content, Status: req.Status}
	if err := ac.DB.Create(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建知识库失败"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"data": item})
}

func (ac *AdminController) UpdateUserKnowledgeBaseAdmin(c *gin.Context) {
	userID, _ := strconv.Atoi(c.Param("id"))
	kbID, _ := strconv.Atoi(c.Param("kb_id"))
	if userID <= 0 || kbID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的参数"})
		return
	}
	var item models.KnowledgeBase
	if err := ac.DB.Where("id = ? AND user_id = ?", kbID, userID).First(&item).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "知识库不存在"})
		return
	}
	var req struct {
		Name        string `json:"name" binding:"required,min=1,max=100"`
		Description string `json:"description"`
		Content     string `json:"content"`
		Status      string `json:"status"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数错误: " + err.Error()})
		return
	}
	item.Name = req.Name
	item.Description = req.Description
	item.Content = req.Content
	if req.Status != "" {
		item.Status = req.Status
	}
	if err := ac.DB.Save(&item).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新知识库失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": item})
}

func (ac *AdminController) DeleteUserKnowledgeBaseAdmin(c *gin.Context) {
	userID, _ := strconv.Atoi(c.Param("id"))
	kbID, _ := strconv.Atoi(c.Param("kb_id"))
	if userID <= 0 || kbID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "无效的参数"})
		return
	}
	err := ac.DB.Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("id = ? AND user_id = ?", kbID, userID).Delete(&models.KnowledgeBase{}).Error; err != nil {
			return err
		}
		return tx.Where("knowledge_base_id = ?", kbID).Delete(&models.AgentKnowledgeBase{}).Error
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除知识库失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}
