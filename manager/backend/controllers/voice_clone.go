package controllers

import (
	"bytes"
	"context"
	crand "crypto/rand"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"xiaozhi/manager/backend/config"
	"xiaozhi/manager/backend/models"
	"xiaozhi/manager/backend/storage"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

type CloneProviderCapability struct {
	Enabled            bool
	RequiresTranscript bool
	MinTextLen         int
	MaxTextLen         int
	SupportedLangs     map[string]bool
}

var cloneProviderCapabilities = map[string]CloneProviderCapability{
	"minimax": {
		Enabled:            true,
		RequiresTranscript: false,
		MinTextLen:         0,
		MaxTextLen:         0,
		SupportedLangs:     map[string]bool{},
	},
}

type VoiceCloneController struct {
	DB           *gorm.DB
	AudioStorage *storage.AudioStorage
	HTTPClient   *http.Client
	taskQueue    chan uint
}

type minimaxVoiceCloneResult struct {
	VoiceID      string
	RawResponse  map[string]any
	RequestID    string
	ResponseCode int
}

const (
	defaultMinimaxCloneEndpoint  = "https://api.minimaxi.com/v1/voice_clone"
	defaultMinimaxUploadEndpoint = "https://api.minimaxi.com/v1/files/upload"
	defaultMinimaxCloneModel     = "speech-2.5-hd-preview"
	minMinimaxCloneAudioSeconds  = 10.0

	voiceCloneStatusQueued     = "queued"
	voiceCloneStatusProcessing = "processing"
	voiceCloneStatusActive     = "active"
	voiceCloneStatusFailed     = "failed"

	voiceCloneTaskStatusQueued     = "queued"
	voiceCloneTaskStatusProcessing = "processing"
	voiceCloneTaskStatusSucceeded  = "succeeded"
	voiceCloneTaskStatusFailed     = "failed"

	voiceCloneTaskQueueSize    = 128
	voiceCloneTaskWorkerCount  = 2
	voiceCloneTaskProcessLimit = 5 * time.Minute
)

var errVoiceCloneQuotaExceeded = errors.New("voice clone quota exceeded")

func NewVoiceCloneController(db *gorm.DB, cfg *config.Config) *VoiceCloneController {
	controller := &VoiceCloneController{
		DB:           db,
		AudioStorage: storage.NewAudioStorage(cfg.Storage.SpeakerAudioPath, cfg.Storage.MaxFileSize),
		HTTPClient: &http.Client{
			Timeout: 90 * time.Second,
		},
		taskQueue: make(chan uint, voiceCloneTaskQueueSize),
	}
	controller.startVoiceCloneWorkers()
	return controller
}

func (vcc *VoiceCloneController) CreateVoiceClone(c *gin.Context) {
	userIDAny, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "认证信息缺失"})
		return
	}
	userID := userIDAny.(uint)

	ttsConfigID := strings.TrimSpace(c.PostForm("tts_config_id"))
	name := strings.TrimSpace(c.PostForm("name"))
	transcript := strings.TrimSpace(c.PostForm("transcript"))
	transcriptLang := strings.TrimSpace(c.DefaultPostForm("transcript_lang", "zh-CN"))
	sourceType := strings.TrimSpace(c.DefaultPostForm("source_type", "upload"))
	if ttsConfigID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "tts_config_id不能为空"})
		return
	}
	if sourceType != "upload" && sourceType != "record" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "source_type仅支持upload或record"})
		return
	}

	var ttsCfg models.Config
	if err := vcc.DB.Where("type = ? AND config_id = ?", "tts", ttsConfigID).First(&ttsCfg).Error; err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "TTS配置不存在"})
		return
	}
	provider := strings.TrimSpace(ttsCfg.Provider)
	if provider != "minimax" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "当前仅支持 Minimax 提供商的声音复刻"})
		return
	}

	capability := GetCloneProviderCapability(provider)
	if !capability.Enabled {
		c.JSON(http.StatusBadRequest, gin.H{"error": "该提供商暂未开启声音复刻"})
		return
	}
	if capability.RequiresTranscript {
		if transcript == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该提供商复刻要求必须填写音频对应文字"})
			return
		}
		if len([]rune(transcript)) < capability.MinTextLen {
			c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("音频对应文字长度不能少于%d个字符", capability.MinTextLen)})
			return
		}
	}
	if capability.MaxTextLen > 0 && len([]rune(transcript)) > capability.MaxTextLen {
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("音频对应文字长度不能超过%d个字符", capability.MaxTextLen)})
		return
	}
	if len(capability.SupportedLangs) > 0 && !capability.SupportedLangs[transcriptLang] {
		c.JSON(http.StatusBadRequest, gin.H{"error": "transcript_lang不受该提供商支持"})
		return
	}

	file, header, err := vcc.pickAudioFile(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	defer file.Close()
	log.Printf("[voice_clone][minimax] incoming audio: source_type=%s filename=%q ext=%q content_type=%q header_size=%d",
		sourceType,
		header.Filename,
		strings.ToLower(filepath.Ext(header.Filename)),
		header.Header.Get("Content-Type"),
		header.Size,
	)

	if name == "" {
		base := strings.TrimSuffix(header.Filename, filepath.Ext(header.Filename))
		if base == "" {
			base = "minimax-voice"
		}
		name = base
	}

	audioUUID := uuid.New().String()
	filePath, size, err := vcc.AudioStorage.SaveVoiceCloneAudioFile(userID, audioUUID, header.Filename, file)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存复刻音频失败: " + err.Error()})
		return
	}
	audioSeconds, err := getMinimaxCloneAudioDurationSeconds(filePath)
	if err != nil {
		_ = vcc.AudioStorage.DeleteAudioFile(filePath)
		c.JSON(http.StatusBadRequest, gin.H{"error": "音频格式校验失败: " + err.Error()})
		return
	}
	log.Printf("[voice_clone][minimax] local duration check: file=%q duration=%.3fs min=%.1fs", filePath, audioSeconds, minMinimaxCloneAudioSeconds)
	if audioSeconds < minMinimaxCloneAudioSeconds {
		_ = vcc.AudioStorage.DeleteAudioFile(filePath)
		c.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("Minimax 声音复刻要求音频时长至少 %.0f 秒，当前 %.2f 秒", minMinimaxCloneAudioSeconds, audioSeconds)})
		return
	}

	taskID := uuid.New().String()
	providerVoiceID := buildMinimaxCustomVoiceID(ttsConfigID)
	pendingMetaJSON, _ := json.Marshal(gin.H{
		"source_type": sourceType,
		"task_id":     taskID,
		"task_status": voiceCloneTaskStatusQueued,
		"queued_at":   time.Now(),
	})

	clone := models.VoiceClone{
		UserID:             userID,
		Name:               name,
		Provider:           provider,
		ProviderVoiceID:    providerVoiceID,
		TTSConfigID:        ttsConfigID,
		Status:             voiceCloneStatusProcessing,
		TranscriptRequired: capability.RequiresTranscript,
		MetaJSON:           string(pendingMetaJSON),
	}
	audio := models.VoiceCloneAudio{
		UserID:         userID,
		SourceType:     sourceType,
		FilePath:       filePath,
		FileName:       header.Filename,
		FileSize:       size,
		ContentType:    header.Header.Get("Content-Type"),
		Transcript:     transcript,
		TranscriptLang: transcriptLang,
	}

	task := models.VoiceCloneTask{
		TaskID:    taskID,
		UserID:    userID,
		Provider:  provider,
		Status:    voiceCloneTaskStatusQueued,
		Attempts:  0,
		LastError: "",
	}

	err = vcc.DB.Transaction(func(tx *gorm.DB) error {
		if err = vcc.consumeVoiceCloneQuota(tx, userID, ttsConfigID); err != nil {
			return err
		}
		if err = tx.Create(&clone).Error; err != nil {
			return err
		}
		audio.VoiceCloneID = &clone.ID
		if err = tx.Create(&audio).Error; err != nil {
			return err
		}
		task.VoiceCloneID = clone.ID
		if err = tx.Create(&task).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		_ = vcc.AudioStorage.DeleteAudioFile(filePath)
		if errors.Is(err, errVoiceCloneQuotaExceeded) {
			c.JSON(http.StatusBadRequest, gin.H{"error": "该 TTS 配置的声音复刻次数已用完，请联系管理员分配额度"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建复刻任务失败: " + err.Error()})
		return
	}

	vcc.enqueueVoiceCloneTask(task.ID)
	c.JSON(http.StatusAccepted, gin.H{"success": true, "data": gin.H{
		"id": clone.ID, "name": clone.Name, "provider": clone.Provider,
		"provider_voice_id": clone.ProviderVoiceID, "tts_config_id": clone.TTSConfigID,
		"audio_id": audio.ID, "created_at": clone.CreatedAt, "status": clone.Status,
		"task_id": task.TaskID, "task_status": task.Status,
	}})
}

func (vcc *VoiceCloneController) GetVoiceClones(c *gin.Context) {
	userIDAny, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "认证信息缺失"})
		return
	}
	userID := userIDAny.(uint)

	ttsConfigID := strings.TrimSpace(c.Query("tts_config_id"))
	query := vcc.DB.Model(&models.VoiceClone{}).Where("user_id = ? AND status != ?", userID, "deleted")
	if ttsConfigID != "" {
		query = query.Where("tts_config_id = ?", ttsConfigID)
	}

	var clones []models.VoiceClone
	if err := query.Order("created_at DESC").Find(&clones).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询复刻音色失败"})
		return
	}
	if len(clones) == 0 {
		c.JSON(http.StatusOK, gin.H{"data": []gin.H{}})
		return
	}

	cloneIDs := make([]uint, 0, len(clones))
	ttsConfigIDSet := make(map[string]bool, len(clones))
	for _, clone := range clones {
		cloneIDs = append(cloneIDs, clone.ID)
		if strings.TrimSpace(clone.TTSConfigID) != "" {
			ttsConfigIDSet[clone.TTSConfigID] = true
		}
	}

	var tasks []models.VoiceCloneTask
	if err := vcc.DB.Where("user_id = ? AND voice_clone_id IN ?", userID, cloneIDs).Order("created_at DESC").Find(&tasks).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询复刻任务失败"})
		return
	}
	latestTaskByCloneID := make(map[uint]models.VoiceCloneTask, len(clones))
	for _, task := range tasks {
		if _, exists := latestTaskByCloneID[task.VoiceCloneID]; exists {
			continue
		}
		latestTaskByCloneID[task.VoiceCloneID] = task
	}

	ttsConfigNames := make(map[string]string, len(ttsConfigIDSet))
	ttsConfigProviders := make(map[string]string, len(ttsConfigIDSet))
	if len(ttsConfigIDSet) > 0 {
		ttsConfigIDs := make([]string, 0, len(ttsConfigIDSet))
		for configID := range ttsConfigIDSet {
			ttsConfigIDs = append(ttsConfigIDs, configID)
		}
		var ttsConfigs []models.Config
		if err := vcc.DB.Where("type = ? AND config_id IN ?", "tts", ttsConfigIDs).Find(&ttsConfigs).Error; err == nil {
			for _, ttsConfig := range ttsConfigs {
				ttsConfigNames[ttsConfig.ConfigID] = strings.TrimSpace(ttsConfig.Name)
				ttsConfigProviders[ttsConfig.ConfigID] = strings.TrimSpace(ttsConfig.Provider)
			}
		}
	}

	result := make([]gin.H, 0, len(clones))
	for _, clone := range clones {
		item := gin.H{
			"id":                  clone.ID,
			"user_id":             clone.UserID,
			"name":                clone.Name,
			"provider":            clone.Provider,
			"provider_voice_id":   clone.ProviderVoiceID,
			"tts_config_id":       clone.TTSConfigID,
			"tts_config_name":     clone.TTSConfigID,
			"status":              clone.Status,
			"transcript_required": clone.TranscriptRequired,
			"meta_json":           clone.MetaJSON,
			"created_at":          clone.CreatedAt,
			"updated_at":          clone.UpdatedAt,
		}
		if name, ok := ttsConfigNames[clone.TTSConfigID]; ok && name != "" {
			item["tts_config_name"] = name
		}
		if provider, ok := ttsConfigProviders[clone.TTSConfigID]; ok && provider != "" {
			item["tts_provider"] = provider
		}
		if task, ok := latestTaskByCloneID[clone.ID]; ok {
			item["task_id"] = task.TaskID
			item["task_status"] = task.Status
			item["task_attempts"] = task.Attempts
			item["task_last_error"] = task.LastError
			item["task_started_at"] = task.StartedAt
			item["task_finished_at"] = task.FinishedAt
		}
		result = append(result, item)
	}

	c.JSON(http.StatusOK, gin.H{"data": result})
}

func (vcc *VoiceCloneController) UpdateVoiceClone(c *gin.Context) {
	userIDAny, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "认证信息缺失"})
		return
	}
	userID := userIDAny.(uint)

	cloneID := strings.TrimSpace(c.Param("id"))
	if cloneID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "复刻音色ID不能为空"})
		return
	}

	var req struct {
		Name string `json:"name"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "请求参数格式错误"})
		return
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称不能为空"})
		return
	}
	if len([]rune(name)) > 100 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "名称长度不能超过100个字符"})
		return
	}

	var clone models.VoiceClone
	if err := vcc.DB.Where("id = ? AND user_id = ? AND status != ?", cloneID, userID, "deleted").First(&clone).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": "复刻音色不存在"})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询复刻音色失败"})
		return
	}

	if err := vcc.DB.Model(&clone).Update("name", name).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新复刻名称失败"})
		return
	}
	clone.Name = name
	c.JSON(http.StatusOK, gin.H{"success": true, "data": clone})
}

func (vcc *VoiceCloneController) GetVoiceCloneAudios(c *gin.Context) {
	userIDAny, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "认证信息缺失"})
		return
	}
	userID := userIDAny.(uint)

	cloneID := strings.TrimSpace(c.Param("id"))
	var clone models.VoiceClone
	if err := vcc.DB.Where("id = ? AND user_id = ?", cloneID, userID).First(&clone).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "复刻音色不存在"})
		return
	}

	var audios []models.VoiceCloneAudio
	if err := vcc.DB.Where("voice_clone_id = ? AND user_id = ?", clone.ID, userID).Order("created_at DESC").Find(&audios).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "查询复刻音频失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": audios})
}

func (vcc *VoiceCloneController) GetVoiceCloneAudioFile(c *gin.Context) {
	userIDAny, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, gin.H{"error": "认证信息缺失"})
		return
	}
	userID := userIDAny.(uint)

	audioID := strings.TrimSpace(c.Param("audio_id"))
	var audio models.VoiceCloneAudio
	if err := vcc.DB.Where("id = ? AND user_id = ?", audioID, userID).First(&audio).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "复刻音频不存在"})
		return
	}
	if !vcc.AudioStorage.FileExists(audio.FilePath) {
		c.JSON(http.StatusNotFound, gin.H{"error": "音频文件不存在"})
		return
	}

	contentType := audio.ContentType
	if contentType == "" {
		contentType = "audio/wav"
	}
	c.Header("Content-Type", contentType)
	c.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", audio.FileName))
	c.File(audio.FilePath)
}

func (vcc *VoiceCloneController) GetCloneProviderCapabilities(c *gin.Context) {
	provider := strings.TrimSpace(c.Query("provider"))
	if provider == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider参数必填"})
		return
	}
	capability := GetCloneProviderCapability(provider)
	c.JSON(http.StatusOK, gin.H{"data": gin.H{
		"provider":            provider,
		"enabled":             capability.Enabled,
		"requires_transcript": capability.RequiresTranscript,
		"min_text_len":        capability.MinTextLen,
		"max_text_len":        capability.MaxTextLen,
		"supported_langs":     mapsKeys(capability.SupportedLangs),
		"updated_at":          time.Now(),
	}})
}

func (vcc *VoiceCloneController) pickAudioFile(c *gin.Context) (multipart.File, *multipart.FileHeader, error) {
	candidates := []string{"audio_file", "audio_blob", "audio"}
	for _, field := range candidates {
		file, header, err := c.Request.FormFile(field)
		if err == nil {
			return file, header, nil
		}
	}
	return nil, nil, fmt.Errorf("请上传音频文件（audio_file 或 audio_blob）")
}

func GetCloneProviderCapability(provider string) CloneProviderCapability {
	if capability, ok := cloneProviderCapabilities[provider]; ok {
		return capability
	}
	return CloneProviderCapability{Enabled: false, SupportedLangs: map[string]bool{}}
}

func BuildVoiceOptionForClone(clone models.VoiceClone) VoiceOption {
	label := fmt.Sprintf("[我的复刻] %s (%s)", clone.Name, clone.ProviderVoiceID)
	return VoiceOption{Value: clone.ProviderVoiceID, Label: label}
}

func mapsKeys(m map[string]bool) []string {
	result := make([]string, 0, len(m))
	for k, enabled := range m {
		if enabled {
			result = append(result, k)
		}
	}
	return result
}

func (vcc *VoiceCloneController) consumeVoiceCloneQuota(tx *gorm.DB, userID uint, ttsConfigID string) error {
	ttsConfigID = strings.TrimSpace(ttsConfigID)
	if ttsConfigID == "" {
		return nil
	}

	var quota models.UserVoiceCloneQuota
	err := tx.Where("user_id = ? AND tts_config_id = ?", userID, ttsConfigID).First(&quota).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			// 未配置额度时默认不限制，兼容历史行为
			return nil
		}
		return err
	}

	if quota.MaxCount < 0 {
		return nil
	}
	result := tx.Model(&models.UserVoiceCloneQuota{}).
		Where("id = ? AND max_count >= 0 AND used_count < max_count", quota.ID).
		Update("used_count", gorm.Expr("used_count + ?", 1))
	if result.Error != nil {
		return result.Error
	}
	if result.RowsAffected == 0 {
		return errVoiceCloneQuotaExceeded
	}
	return nil
}

func (vcc *VoiceCloneController) cloneWithMinimax(ctx context.Context, ttsCfg models.Config, ttsConfigID, filePath, fileName, transcript string) (*minimaxVoiceCloneResult, error) {
	cfgMap := make(map[string]any)
	if ttsCfg.JsonData != "" {
		if err := json.Unmarshal([]byte(ttsCfg.JsonData), &cfgMap); err != nil {
			return nil, fmt.Errorf("解析TTS配置失败: %w", err)
		}
	}
	apiKey := normalizeMinimaxAPIKey(getStringAny(cfgMap, "api_key"))
	if apiKey == "" {
		return nil, errors.New("minimax api_key 未配置")
	}
	endpoint := strings.TrimSpace(getStringAny(cfgMap, "voice_clone_endpoint", "clone_endpoint"))
	if endpoint == "" {
		endpoint = defaultMinimaxCloneEndpoint
	}
	uploadEndpoint := strings.TrimSpace(getStringAny(cfgMap, "voice_clone_upload_endpoint", "files_upload_endpoint", "file_upload_endpoint"))
	if uploadEndpoint == "" {
		uploadEndpoint = defaultMinimaxUploadEndpoint
	}
	model := strings.TrimSpace(getStringAny(cfgMap, "voice_clone_model", "voice_clone_model_id", "model"))
	if model == "" {
		model = defaultMinimaxCloneModel
	}
	voiceID := buildMinimaxCustomVoiceID(ttsConfigID)
	groupID := strings.TrimSpace(getStringAny(cfgMap, "group_id", "GroupId"))
	log.Printf("[voice_clone][minimax] prepare request: upload_endpoint=%s clone_endpoint=%s model=%q voice_id=%q transcript_len=%d group_id=%q file_name=%q file_path=%q api_key=%s",
		uploadEndpoint,
		endpoint,
		model,
		voiceID,
		len([]rune(strings.TrimSpace(transcript))),
		groupID,
		fileName,
		filePath,
		maskSecret(apiKey),
	)
	return vcc.cloneWithMinimaxEndpoints(ctx, apiKey, endpoint, uploadEndpoint, groupID, filePath, fileName, transcript, model, voiceID)
}

func (vcc *VoiceCloneController) cloneWithMinimaxEndpoints(ctx context.Context, apiKey, cloneEndpoint, uploadEndpoint, groupID, filePath, fileName, transcript, model, voiceID string) (*minimaxVoiceCloneResult, error) {
	fileID, err := vcc.uploadMinimaxVoiceCloneFile(ctx, apiKey, uploadEndpoint, groupID, filePath, fileName)
	if err != nil {
		return nil, err
	}
	fileIDPayload := makeMinimaxFileIDPayload(fileID)

	bodyMap := map[string]any{
		"file_id":  fileIDPayload,
		"voice_id": voiceID,
	}
	transcript = strings.TrimSpace(transcript)
	if transcript != "" {
		bodyMap["text"] = transcript
		if model != "" {
			bodyMap["model"] = model
		}
	}

	bodyBytes, err := json.Marshal(bodyMap)
	if err != nil {
		return nil, fmt.Errorf("构建Minimax复刻请求失败: %w", err)
	}
	log.Printf("[voice_clone][minimax] clone request: endpoint=%s file_id_type=%T body=%s group_id=%q api_key=%s",
		cloneEndpoint,
		fileIDPayload,
		truncateForLog(string(bodyBytes), 1024),
		groupID,
		maskSecret(apiKey),
	)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, cloneEndpoint, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if groupID != "" {
		req.Header.Set("Group-Id", groupID)
		req.Header.Set("GroupId", groupID)
	}

	resp, err := vcc.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return nil, fmt.Errorf("读取响应失败: %w", err)
	}
	log.Printf("[voice_clone][minimax] clone response: status=%d body=%s",
		resp.StatusCode,
		truncateForLog(strings.TrimSpace(string(respBody)), 4096),
	)
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	parsed, err := unmarshalJSONMap(respBody)
	if err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}
	if statusCode, statusMsg, ok := parseMinimaxStatus(parsed); ok && statusCode != 0 {
		return nil, fmt.Errorf("Minimax返回错误(code=%d, msg=%s): %s", statusCode, statusMsg, strings.TrimSpace(string(respBody)))
	}

	resolvedVoiceID := strings.TrimSpace(voiceID)
	if resolvedVoiceID == "" {
		return nil, errors.New("请求 voice_id 为空")
	}
	if payloadVoiceID := pickVoiceID(parsed); payloadVoiceID != "" && payloadVoiceID != resolvedVoiceID {
		log.Printf("[voice_clone][minimax] clone response voice_id=%q ignored, using requested voice_id=%q", payloadVoiceID, resolvedVoiceID)
	}
	if pickVoiceID(parsed) == "" {
		log.Printf("[voice_clone][minimax] clone response missing voice_id, using requested voice_id=%q", resolvedVoiceID)
	}
	return &minimaxVoiceCloneResult{
		VoiceID:      resolvedVoiceID,
		RawResponse:  parsed,
		RequestID:    getStringAny(parsed, "request_id", "trace_id"),
		ResponseCode: resp.StatusCode,
	}, nil
}

func (vcc *VoiceCloneController) uploadMinimaxVoiceCloneFile(ctx context.Context, apiKey, uploadEndpoint, groupID, filePath, fileName string) (string, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return "", fmt.Errorf("读取音频文件失败: %w", err)
	}
	defer f.Close()
	fileSize := int64(-1)
	if stat, statErr := f.Stat(); statErr == nil {
		fileSize = stat.Size()
	}
	detectedContentType := ""
	if _, seekErr := f.Seek(0, io.SeekStart); seekErr == nil {
		sniffBuf := make([]byte, 512)
		n, readErr := f.Read(sniffBuf)
		if readErr == nil || readErr == io.EOF {
			if n > 0 {
				detectedContentType = http.DetectContentType(sniffBuf[:n])
			}
		}
		_, _ = f.Seek(0, io.SeekStart)
	}
	log.Printf("[voice_clone][minimax] upload request: endpoint=%s purpose=voice_clone file_name=%q file_ext=%q stored_ext=%q file_size=%d detected_content_type=%q group_id=%q api_key=%s",
		uploadEndpoint,
		fileName,
		strings.ToLower(filepath.Ext(fileName)),
		strings.ToLower(filepath.Ext(filePath)),
		fileSize,
		detectedContentType,
		groupID,
		maskSecret(apiKey),
	)

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	if err = writer.WriteField("purpose", "voice_clone"); err != nil {
		return "", fmt.Errorf("构建上传参数失败: %w", err)
	}
	formFile, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return "", fmt.Errorf("创建上传文件表单失败: %w", err)
	}
	if _, err = io.Copy(formFile, f); err != nil {
		return "", fmt.Errorf("写入上传文件失败: %w", err)
	}
	if err = writer.Close(); err != nil {
		return "", fmt.Errorf("构建上传请求失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, uploadEndpoint, &body)
	if err != nil {
		return "", fmt.Errorf("创建上传请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	if groupID != "" {
		req.Header.Set("Group-Id", groupID)
		req.Header.Set("GroupId", groupID)
	}

	resp, err := vcc.HTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("上传复刻音频失败: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 2*1024*1024))
	if err != nil {
		return "", fmt.Errorf("读取上传响应失败: %w", err)
	}
	log.Printf("[voice_clone][minimax] upload response: status=%d body=%s",
		resp.StatusCode,
		truncateForLog(strings.TrimSpace(string(respBody)), 4096),
	)
	if resp.StatusCode >= 400 {
		return "", fmt.Errorf("上传复刻音频HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	parsed, err := unmarshalJSONMap(respBody)
	if err != nil {
		return "", fmt.Errorf("解析上传响应失败: %w", err)
	}
	if statusCode, statusMsg, ok := parseMinimaxStatus(parsed); ok && statusCode != 0 {
		return "", fmt.Errorf("上传复刻音频被Minimax拒绝(code=%d, msg=%s): %s", statusCode, statusMsg, strings.TrimSpace(string(respBody)))
	}

	fileMap, ok := parsed["file"].(map[string]any)
	if !ok {
		return "", fmt.Errorf("上传响应中未返回 file 对象: %s", strings.TrimSpace(string(respBody)))
	}
	fileID := getStringOrNumberAny(fileMap, "file_id", "fileId", "id")
	if fileID == "" {
		return "", fmt.Errorf("上传响应中未返回 file_id: %s", strings.TrimSpace(string(respBody)))
	}
	return fileID, nil
}

func pickVoiceID(payload map[string]any) string {
	candidates := []string{"voice_id", "voiceId", "voice", "speaker_id", "speakerId"}
	for _, key := range candidates {
		if value := getStringAny(payload, key); value != "" {
			return value
		}
	}
	if data, ok := payload["data"].(map[string]any); ok {
		for _, key := range candidates {
			if value := getStringAny(data, key); value != "" {
				return value
			}
		}
	}
	return ""
}

func parseMinimaxStatus(payload map[string]any) (int, string, bool) {
	baseResp, ok := payload["base_resp"].(map[string]any)
	if !ok {
		return 0, "", false
	}
	code, ok := getIntAny(baseResp, "status_code")
	if !ok {
		return 0, "", false
	}
	return code, strings.TrimSpace(getStringAny(baseResp, "status_msg")), true
}

func normalizeMinimaxAPIKey(raw string) string {
	key := strings.TrimSpace(strings.Trim(raw, "\"'"))
	if key == "" {
		return ""
	}
	lowerKey := strings.ToLower(key)
	if strings.HasPrefix(lowerKey, "bearer ") {
		key = strings.TrimSpace(key[len("bearer "):])
	}
	return strings.TrimSpace(strings.Trim(key, "\"'"))
}

func maskSecret(secret string) string {
	secret = strings.TrimSpace(secret)
	if secret == "" {
		return "<empty>"
	}
	if len(secret) <= 8 {
		return fmt.Sprintf("%s(len=%d)", strings.Repeat("*", len(secret)), len(secret))
	}
	return fmt.Sprintf("%s...%s(len=%d)", secret[:4], secret[len(secret)-4:], len(secret))
}

func truncateForLog(s string, max int) string {
	if max <= 0 {
		return ""
	}
	if len(s) <= max {
		return s
	}
	return s[:max] + "...(truncated)"
}

func getMinimaxCloneAudioDurationSeconds(filePath string) (float64, error) {
	ext := strings.ToLower(strings.TrimSpace(filepath.Ext(filePath)))
	if ext != ".wav" {
		return 0, fmt.Errorf("当前仅支持 WAV 音频，检测到扩展名: %s", ext)
	}
	return getWAVDurationSeconds(filePath)
}

func getWAVDurationSeconds(filePath string) (float64, error) {
	f, err := os.Open(filePath)
	if err != nil {
		return 0, fmt.Errorf("打开音频文件失败: %w", err)
	}
	defer f.Close()

	header := make([]byte, 12)
	if _, err = io.ReadFull(f, header); err != nil {
		return 0, fmt.Errorf("读取WAV头失败: %w", err)
	}
	if string(header[0:4]) != "RIFF" || string(header[8:12]) != "WAVE" {
		return 0, errors.New("不是有效的 WAV 文件")
	}

	var sampleRate uint32
	var channels uint16
	var bitsPerSample uint16
	var dataBytes uint64

	for {
		chunkHeader := make([]byte, 8)
		_, err = io.ReadFull(f, chunkHeader)
		if err == io.EOF {
			break
		}
		if err != nil {
			if errors.Is(err, io.ErrUnexpectedEOF) {
				break
			}
			return 0, fmt.Errorf("读取WAV分块头失败: %w", err)
		}
		chunkID := string(chunkHeader[0:4])
		chunkSize := binary.LittleEndian.Uint32(chunkHeader[4:8])
		chunkSizeInt := int64(chunkSize)

		switch chunkID {
		case "fmt ":
			if chunkSize < 16 {
				return 0, fmt.Errorf("WAV fmt 分块长度无效: %d", chunkSize)
			}
			fmtData := make([]byte, chunkSize)
			if _, err = io.ReadFull(f, fmtData); err != nil {
				return 0, fmt.Errorf("读取WAV fmt分块失败: %w", err)
			}
			audioFormat := binary.LittleEndian.Uint16(fmtData[0:2])
			if audioFormat != 1 && audioFormat != 3 {
				return 0, fmt.Errorf("不支持的 WAV 编码格式: %d", audioFormat)
			}
			channels = binary.LittleEndian.Uint16(fmtData[2:4])
			sampleRate = binary.LittleEndian.Uint32(fmtData[4:8])
			bitsPerSample = binary.LittleEndian.Uint16(fmtData[14:16])
		case "data":
			dataBytes = uint64(chunkSize)
			if _, err = f.Seek(chunkSizeInt, io.SeekCurrent); err != nil {
				return 0, fmt.Errorf("跳过WAV data分块失败: %w", err)
			}
		default:
			if _, err = f.Seek(chunkSizeInt, io.SeekCurrent); err != nil {
				return 0, fmt.Errorf("跳过WAV分块失败: %w", err)
			}
		}

		// WAV chunk 数据按 2 字节对齐，奇数长度需补 1 字节。
		if chunkSize%2 == 1 {
			if _, err = f.Seek(1, io.SeekCurrent); err != nil {
				return 0, fmt.Errorf("跳过WAV对齐字节失败: %w", err)
			}
		}
	}

	if sampleRate == 0 || channels == 0 || bitsPerSample == 0 || dataBytes == 0 {
		return 0, fmt.Errorf("WAV信息不完整(sample_rate=%d channels=%d bits=%d data_bytes=%d)", sampleRate, channels, bitsPerSample, dataBytes)
	}
	bytesPerSecond := (float64(sampleRate) * float64(channels) * float64(bitsPerSample)) / 8.0
	if bytesPerSecond <= 0 {
		return 0, errors.New("WAV每秒字节数无效")
	}
	return float64(dataBytes) / bytesPerSecond, nil
}

func buildMinimaxCustomVoiceID(ttsConfigID string) string {
	prefix := sanitizeMinimaxVoiceIDPrefix(ttsConfigID)
	return fmt.Sprintf("%s_%s", prefix, randomDigits(8))
}

func sanitizeMinimaxVoiceIDPrefix(ttsConfigID string) string {
	ttsConfigID = strings.TrimSpace(ttsConfigID)
	if ttsConfigID == "" {
		return "voice"
	}
	filtered := make([]byte, 0, len(ttsConfigID))
	for i := 0; i < len(ttsConfigID); i++ {
		ch := ttsConfigID[i]
		if (ch >= 'A' && ch <= 'Z') || (ch >= 'a' && ch <= 'z') || (ch >= '0' && ch <= '9') || ch == '_' {
			filtered = append(filtered, ch)
			continue
		}
		filtered = append(filtered, '_')
	}
	prefix := strings.Trim(strings.TrimSpace(string(filtered)), "_")
	if prefix == "" {
		return "voice"
	}
	return prefix
}

func randomDigits(n int) string {
	if n <= 0 {
		return ""
	}
	buf := make([]byte, n)
	if _, err := io.ReadFull(crand.Reader, buf); err == nil {
		for i := range buf {
			buf[i] = '0' + (buf[i] % 10)
		}
		return string(buf)
	}
	fallback := fmt.Sprintf("%d", time.Now().UnixNano())
	if len(fallback) >= n {
		return fallback[len(fallback)-n:]
	}
	if len(fallback) == 0 {
		return strings.Repeat("0", n)
	}
	return strings.Repeat("0", n-len(fallback)) + fallback
}

func getStringAny(m map[string]any, keys ...string) string {
	for _, key := range keys {
		raw, ok := m[key]
		if !ok {
			continue
		}
		if value, ok := raw.(string); ok {
			value = strings.TrimSpace(value)
			if value != "" {
				return value
			}
		}
	}
	return ""
}

func getStringOrNumberAny(m map[string]any, keys ...string) string {
	if value := getStringAny(m, keys...); value != "" {
		return value
	}
	for _, key := range keys {
		raw, ok := m[key]
		if !ok || raw == nil {
			continue
		}
		switch value := raw.(type) {
		case json.Number:
			value = json.Number(strings.TrimSpace(string(value)))
			if value == "" {
				continue
			}
			return value.String()
		case int:
			return strconv.Itoa(value)
		case int8:
			return strconv.FormatInt(int64(value), 10)
		case int16:
			return strconv.FormatInt(int64(value), 10)
		case int32:
			return strconv.FormatInt(int64(value), 10)
		case int64:
			return strconv.FormatInt(value, 10)
		case uint:
			return strconv.FormatUint(uint64(value), 10)
		case uint8:
			return strconv.FormatUint(uint64(value), 10)
		case uint16:
			return strconv.FormatUint(uint64(value), 10)
		case uint32:
			return strconv.FormatUint(uint64(value), 10)
		case uint64:
			return strconv.FormatUint(value, 10)
		case float32:
			return strconv.FormatFloat(float64(value), 'f', -1, 32)
		case float64:
			return strconv.FormatFloat(value, 'f', -1, 64)
		}
	}
	return ""
}

func makeMinimaxFileIDPayload(fileID string) any {
	fileID = strings.TrimSpace(fileID)
	if fileID == "" {
		return ""
	}
	if _, err := strconv.ParseInt(fileID, 10, 64); err == nil {
		return json.Number(fileID)
	}
	return fileID
}

func unmarshalJSONMap(payload []byte) (map[string]any, error) {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.UseNumber()
	var parsed map[string]any
	if err := decoder.Decode(&parsed); err != nil {
		return nil, err
	}
	return parsed, nil
}

func getIntAny(m map[string]any, keys ...string) (int, bool) {
	for _, key := range keys {
		raw, ok := m[key]
		if !ok {
			continue
		}
		switch value := raw.(type) {
		case int:
			return value, true
		case int8:
			return int(value), true
		case int16:
			return int(value), true
		case int32:
			return int(value), true
		case int64:
			return int(value), true
		case uint:
			return int(value), true
		case uint8:
			return int(value), true
		case uint16:
			return int(value), true
		case uint32:
			return int(value), true
		case uint64:
			return int(value), true
		case float32:
			return int(value), true
		case float64:
			return int(value), true
		case json.Number:
			n, err := value.Int64()
			if err == nil {
				return int(n), true
			}
		case string:
			n, err := strconv.Atoi(strings.TrimSpace(value))
			if err == nil {
				return n, true
			}
		}
	}
	return 0, false
}
