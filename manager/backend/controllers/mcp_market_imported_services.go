package controllers

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"xiaozhi/manager/backend/models"
	mcpmarket "xiaozhi/manager/backend/services/mcp_market"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type upsertMCPMarketImportedServiceRequest struct {
	Name        string            `json:"name"`
	Enabled     *bool             `json:"enabled"`
	Transport   string            `json:"transport" binding:"required"`
	URL         string            `json:"url" binding:"required"`
	Headers     map[string]string `json:"headers"`
	MarketID    *uint             `json:"market_id"`
	ProviderID  string            `json:"provider_id"`
	ServiceID   string            `json:"service_id"`
	ServiceName string            `json:"service_name"`
}

type mcpMarketImportedServiceView struct {
	ID          uint              `json:"id"`
	Name        string            `json:"name"`
	Enabled     bool              `json:"enabled"`
	Transport   string            `json:"transport"`
	URL         string            `json:"url"`
	Headers     map[string]string `json:"headers,omitempty"`
	MarketID    *uint             `json:"market_id,omitempty"`
	ProviderID  string            `json:"provider_id,omitempty"`
	ServiceID   string            `json:"service_id,omitempty"`
	ServiceName string            `json:"service_name,omitempty"`
	CreatedAt   string            `json:"created_at"`
	UpdatedAt   string            `json:"updated_at"`
}

func (ac *AdminController) GetMCPMarketImportedServices(c *gin.Context) {
	queryText := strings.TrimSpace(c.Query("q"))
	page := parsePositiveInt(c.Query("page"), 1)
	pageSize := parsePositiveInt(c.Query("page_size"), 20)
	if pageSize > 100 {
		pageSize = 100
	}

	db := ac.DB.Model(&models.MCPMarketService{})
	if queryText != "" {
		like := "%" + queryText + "%"
		db = db.Where("name LIKE ? OR service_id LIKE ? OR service_name LIKE ? OR url LIKE ?", like, like, like, like)
	}

	var total int64
	if err := db.Count(&total).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询导入服务总数失败"})
		return
	}

	var rows []models.MCPMarketService
	offset := (page - 1) * pageSize
	if err := db.Order("updated_at DESC, id DESC").Limit(pageSize).Offset(offset).Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询导入服务列表失败"})
		return
	}

	items := make([]mcpMarketImportedServiceView, 0, len(rows))
	for _, row := range rows {
		items = append(items, toMCPMarketImportedServiceView(row))
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"items":     items,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	}})
}

func (ac *AdminController) CreateMCPMarketImportedService(c *gin.Context) {
	var req upsertMCPMarketImportedServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	model, err := buildImportedServiceModelFromRequest(req, nil)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var existing models.MCPMarketService
	if err := ac.DB.Where("url_hash = ?", model.URLHash).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "已存在相同 URL 的导入服务"})
		return
	} else if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询导入服务失败"})
		return
	}

	if err := ac.DB.Create(&model).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建导入服务失败"})
		return
	}

	ac.notifySystemConfigChanged()
	c.JSON(http.StatusCreated, gin.H{"data": toMCPMarketImportedServiceView(model)})
}

func (ac *AdminController) UpdateMCPMarketImportedService(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))

	var existing models.MCPMarketService
	if err := ac.DB.First(&existing, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "导入服务不存在"})
		return
	}

	var req upsertMCPMarketImportedServiceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	updated, err := buildImportedServiceModelFromRequest(req, &existing)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var dup models.MCPMarketService
	if err := ac.DB.Where("id != ? AND url_hash = ?", existing.ID, updated.URLHash).First(&dup).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{"error": "已存在相同 URL 的导入服务"})
		return
	} else if err != nil && err != gorm.ErrRecordNotFound {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询导入服务失败"})
		return
	}

	updateMap := map[string]interface{}{
		"name":         updated.Name,
		"enabled":      updated.Enabled,
		"transport":    updated.Transport,
		"url":          updated.URL,
		"url_hash":     updated.URLHash,
		"headers_json": updated.HeadersJSON,
		"market_id":    updated.MarketID,
		"provider_id":  updated.ProviderID,
		"service_id":   updated.ServiceID,
		"service_name": updated.ServiceName,
	}
	if err := ac.DB.Model(&existing).Updates(updateMap).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新导入服务失败"})
		return
	}

	if err := ac.DB.First(&existing, id).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "读取更新后的导入服务失败"})
		return
	}

	ac.notifySystemConfigChanged()
	c.JSON(http.StatusOK, gin.H{"data": toMCPMarketImportedServiceView(existing)})
}

func (ac *AdminController) DeleteMCPMarketImportedService(c *gin.Context) {
	id, _ := strconv.Atoi(c.Param("id"))
	var existing models.MCPMarketService
	if err := ac.DB.First(&existing, id).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "导入服务不存在"})
		return
	}

	if err := ac.DB.Delete(&existing).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除导入服务失败"})
		return
	}

	ac.notifySystemConfigChanged()
	c.JSON(http.StatusOK, gin.H{"message": "删除成功"})
}

func (ac *AdminController) listMCPMarketServices(enabledOnly bool) ([]models.MCPMarketService, error) {
	var rows []models.MCPMarketService
	db := ac.DB.Model(&models.MCPMarketService{})
	if enabledOnly {
		db = db.Where("enabled = ?", true)
	}
	if err := db.Order("id ASC").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func mergeManualAndMarketServers(manualMCP map[string]interface{}, marketServices []models.MCPMarketService) (map[string]interface{}, []string, error) {
	merged := deepCopyMap(manualMCP)
	ensureMCPGlobalDefaults(merged)
	global := asMap(merged["global"])

	manualServers, err := decodeMCPServers(global["servers"])
	if err != nil {
		return nil, nil, fmt.Errorf("解析人工MCP服务失败: %w", err)
	}

	existingURLSet := make(map[string]struct{})
	for _, server := range manualServers {
		norm := normalizeServerURL(server)
		if norm == "" {
			continue
		}
		existingURLSet[norm] = struct{}{}
	}

	warnings := make([]string, 0)
	for _, service := range marketServices {
		if !service.Enabled {
			continue
		}

		normURL := mcpmarket.NormalizeURL(service.URL)
		if normURL == "" {
			continue
		}

		if _, exists := existingURLSet[normURL]; exists {
			warnings = append(warnings, fmt.Sprintf("市场服务 %s 因 URL 与人工配置冲突被跳过", service.Name))
			continue
		}

		transport := normalizeImportedTransport(service.Transport)
		if transport != mcpmarket.TransportSSE && transport != mcpmarket.TransportStreamableHTTP {
			continue
		}

		server := mcpServerConfig{
			Name:      service.Name,
			Type:      transport,
			Url:       service.URL,
			Enabled:   service.Enabled,
			Provider:  "mcp-market",
			ServiceID: service.ServiceID,
			Headers:   decodeHeadersJSON(service.HeadersJSON),
		}
		if transport == mcpmarket.TransportSSE {
			server.SSEUrl = service.URL
		}

		manualServers = append(manualServers, server)
		existingURLSet[normURL] = struct{}{}
	}

	global["servers"] = manualServers
	merged["global"] = global
	return merged, warnings, nil
}

func (ac *AdminController) mergeMCPWithEnabledMarketServices(manualMCP map[string]interface{}) (map[string]interface{}, []string, error) {
	services, err := ac.listMCPMarketServices(true)
	if err != nil {
		return nil, nil, err
	}
	return mergeManualAndMarketServers(manualMCP, services)
}

func filterEnabledMarketServices(rows []models.MCPMarketService) []models.MCPMarketService {
	ret := make([]models.MCPMarketService, 0, len(rows))
	for _, row := range rows {
		if row.Enabled {
			ret = append(ret, row)
		}
	}
	return ret
}

func collectManualURLSet(manualMCP map[string]interface{}) (map[string]struct{}, error) {
	ret := make(map[string]struct{})
	merged := deepCopyMap(manualMCP)
	ensureMCPGlobalDefaults(merged)
	global := asMap(merged["global"])
	servers, err := decodeMCPServers(global["servers"])
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		norm := normalizeServerURL(server)
		if norm == "" {
			continue
		}
		ret[norm] = struct{}{}
	}
	return ret, nil
}

func collectUsedServerNames(manualMCP map[string]interface{}, marketServices []models.MCPMarketService) (map[string]struct{}, error) {
	ret := make(map[string]struct{})
	merged := deepCopyMap(manualMCP)
	ensureMCPGlobalDefaults(merged)
	global := asMap(merged["global"])
	servers, err := decodeMCPServers(global["servers"])
	if err != nil {
		return nil, err
	}
	for _, server := range servers {
		name := strings.TrimSpace(server.Name)
		if name != "" {
			ret[name] = struct{}{}
		}
	}
	for _, item := range marketServices {
		name := strings.TrimSpace(item.Name)
		if name != "" {
			ret[name] = struct{}{}
		}
	}
	return ret, nil
}

func mergeServiceUpserts(base []models.MCPMarketService, upserts []models.MCPMarketService) []models.MCPMarketService {
	m := make(map[string]models.MCPMarketService)
	for _, item := range base {
		if item.URLHash == "" {
			continue
		}
		if item.Enabled {
			m[item.URLHash] = item
		}
	}
	for _, item := range upserts {
		if item.URLHash == "" {
			continue
		}
		if item.Enabled {
			m[item.URLHash] = item
		} else {
			delete(m, item.URLHash)
		}
	}

	ret := make([]models.MCPMarketService, 0, len(m))
	for _, item := range m {
		ret = append(ret, item)
	}
	sort.Slice(ret, func(i, j int) bool {
		return ret[i].ID < ret[j].ID
	})
	return ret
}

func normalizeImportedTransport(transport string) string {
	transport = strings.ToLower(strings.TrimSpace(transport))
	switch transport {
	case "sse":
		return mcpmarket.TransportSSE
	case "streamablehttp", "streamable_http", "streamable-http", "http":
		return mcpmarket.TransportStreamableHTTP
	default:
		return transport
	}
}

func normalizeImportedServiceURL(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return ""
	}
	return mcpmarket.NormalizeURL(raw)
}

func normalizeImportedServiceName(name, serviceName, serviceID, fallbackURL string) string {
	name = strings.TrimSpace(name)
	if name != "" {
		return name
	}
	serviceName = strings.TrimSpace(serviceName)
	if serviceName != "" {
		return serviceName
	}
	serviceID = strings.TrimSpace(serviceID)
	if serviceID != "" {
		return serviceID
	}
	if fallbackURL != "" {
		return fallbackURL
	}
	return "mcp-service"
}

func normalizedURLHash(rawURL string) string {
	norm := normalizeImportedServiceURL(rawURL)
	if norm == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(norm))
	return hex.EncodeToString(sum[:])
}

func cleanHeaders(headers map[string]string) map[string]string {
	if len(headers) == 0 {
		return nil
	}
	out := make(map[string]string)
	for k, v := range headers {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil
	}
	return out
}

func encodeHeadersJSON(headers map[string]string) string {
	headers = cleanHeaders(headers)
	if len(headers) == 0 {
		return ""
	}
	b, err := json.Marshal(headers)
	if err != nil {
		return ""
	}
	return string(b)
}

func decodeHeadersJSON(raw string) map[string]string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	var headers map[string]string
	if err := json.Unmarshal([]byte(raw), &headers); err != nil {
		return nil
	}
	return cleanHeaders(headers)
}

func buildImportedServiceModelFromRequest(req upsertMCPMarketImportedServiceRequest, existing *models.MCPMarketService) (models.MCPMarketService, error) {
	row := models.MCPMarketService{}
	if existing != nil {
		row = *existing
	}

	if req.Enabled != nil {
		row.Enabled = *req.Enabled
	} else if existing == nil {
		row.Enabled = true
	}

	transport := strings.TrimSpace(req.Transport)
	if transport == "" && existing != nil {
		transport = existing.Transport
	}
	transport = normalizeImportedTransport(transport)
	if transport != mcpmarket.TransportSSE && transport != mcpmarket.TransportStreamableHTTP {
		return models.MCPMarketService{}, fmt.Errorf("transport 仅支持 sse/streamablehttp")
	}

	rawURL := strings.TrimSpace(req.URL)
	if rawURL == "" && existing != nil {
		rawURL = existing.URL
	}
	if rawURL == "" {
		return models.MCPMarketService{}, fmt.Errorf("url 不能为空")
	}
	if normalizeImportedServiceURL(rawURL) == "" {
		return models.MCPMarketService{}, fmt.Errorf("url 格式不正确")
	}
	urlHash := normalizedURLHash(rawURL)
	if urlHash == "" {
		return models.MCPMarketService{}, fmt.Errorf("url 不能为空")
	}

	row.Transport = transport
	row.URL = rawURL
	row.URLHash = urlHash
	row.Name = normalizeImportedServiceName(req.Name, req.ServiceName, req.ServiceID, rawURL)
	row.HeadersJSON = encodeHeadersJSON(req.Headers)
	row.MarketID = req.MarketID
	row.ProviderID = mcpmarket.NormalizeProviderID(req.ProviderID)
	row.ServiceID = strings.TrimSpace(req.ServiceID)
	row.ServiceName = strings.TrimSpace(req.ServiceName)

	return row, nil
}

func toMCPMarketImportedServiceView(row models.MCPMarketService) mcpMarketImportedServiceView {
	return mcpMarketImportedServiceView{
		ID:          row.ID,
		Name:        row.Name,
		Enabled:     row.Enabled,
		Transport:   normalizeImportedTransport(row.Transport),
		URL:         row.URL,
		Headers:     decodeHeadersJSON(row.HeadersJSON),
		MarketID:    row.MarketID,
		ProviderID:  row.ProviderID,
		ServiceID:   row.ServiceID,
		ServiceName: row.ServiceName,
		CreatedAt:   row.CreatedAt.Format("2006-01-02 15:04:05"),
		UpdatedAt:   row.UpdatedAt.Format("2006-01-02 15:04:05"),
	}
}
