package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"path/filepath"
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
}

type minimaxVoiceCloneResult struct {
	VoiceID      string
	RawResponse  map[string]any
	RequestID    string
	ResponseCode int
}

func NewVoiceCloneController(db *gorm.DB, cfg *config.Config) *VoiceCloneController {
	return &VoiceCloneController{
		DB:           db,
		AudioStorage: storage.NewAudioStorage(cfg.Storage.SpeakerAudioPath, cfg.Storage.MaxFileSize),
		HTTPClient: &http.Client{
			Timeout: 90 * time.Second,
		},
	}
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

	cloneResult, err := vcc.cloneWithMinimax(c.Request.Context(), ttsCfg, filePath, header.Filename, transcript, transcriptLang)
	if err != nil {
		_ = vcc.AudioStorage.DeleteAudioFile(filePath)
		c.JSON(http.StatusBadGateway, gin.H{"error": "Minimax 声音复刻失败: " + err.Error()})
		return
	}

	metaJSON, _ := json.Marshal(gin.H{
		"source_type": sourceType,
		"request_id":  cloneResult.RequestID,
		"http_code":   cloneResult.ResponseCode,
		"response":    cloneResult.RawResponse,
	})

	clone := models.VoiceClone{
		UserID:             userID,
		Name:               name,
		Provider:           provider,
		ProviderVoiceID:    cloneResult.VoiceID,
		TTSConfigID:        ttsConfigID,
		Status:             "active",
		TranscriptRequired: capability.RequiresTranscript,
		MetaJSON:           string(metaJSON),
	}
	if err := vcc.DB.Create(&clone).Error; err != nil {
		_ = vcc.AudioStorage.DeleteAudioFile(filePath)
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存复刻音色失败: " + err.Error()})
		return
	}

	audio := models.VoiceCloneAudio{
		VoiceCloneID:   &clone.ID,
		UserID:         userID,
		SourceType:     sourceType,
		FilePath:       filePath,
		FileName:       header.Filename,
		FileSize:       size,
		ContentType:    header.Header.Get("Content-Type"),
		Transcript:     transcript,
		TranscriptLang: transcriptLang,
	}
	if err := vcc.DB.Create(&audio).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "保存复刻音频记录失败: " + err.Error()})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"success": true, "data": gin.H{
		"id": clone.ID, "name": clone.Name, "provider": clone.Provider,
		"provider_voice_id": clone.ProviderVoiceID, "tts_config_id": clone.TTSConfigID,
		"audio_id": audio.ID, "created_at": clone.CreatedAt,
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
	c.JSON(http.StatusOK, gin.H{"data": clones})
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

func (vcc *VoiceCloneController) cloneWithMinimax(ctx context.Context, ttsCfg models.Config, filePath, fileName, transcript, transcriptLang string) (*minimaxVoiceCloneResult, error) {
	cfgMap := make(map[string]any)
	if ttsCfg.JsonData != "" {
		if err := json.Unmarshal([]byte(ttsCfg.JsonData), &cfgMap); err != nil {
			return nil, fmt.Errorf("解析TTS配置失败: %w", err)
		}
	}
	apiKey := strings.TrimSpace(getStringAny(cfgMap, "api_key"))
	if apiKey == "" {
		return nil, errors.New("minimax api_key 未配置")
	}
	endpoint := strings.TrimSpace(getStringAny(cfgMap, "voice_clone_endpoint", "clone_endpoint"))
	if endpoint == "" {
		endpoint = "https://api.minimaxi.com/v1/voice_clone"
	}
	model := strings.TrimSpace(getStringAny(cfgMap, "voice_clone_model", "voice_clone_model_id", "model"))
	if model == "" {
		model = "speech-2.8-hd"
	}
	voiceID := strings.TrimSpace(getStringAny(cfgMap, "voice_clone_voice_id", "voice_id"))
	groupID := strings.TrimSpace(getStringAny(cfgMap, "group_id", "GroupId"))

	f, err := os.Open(filePath)
	if err != nil {
		return nil, fmt.Errorf("读取音频文件失败: %w", err)
	}
	defer f.Close()

	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	formFile, err := writer.CreateFormFile("file", fileName)
	if err != nil {
		return nil, fmt.Errorf("创建文件表单失败: %w", err)
	}
	if _, err = io.Copy(formFile, f); err != nil {
		return nil, fmt.Errorf("写入音频文件失败: %w", err)
	}
	_ = writer.WriteField("text", transcript)
	_ = writer.WriteField("model", model)
	_ = writer.WriteField("language", transcriptLang)
	if voiceID != "" {
		_ = writer.WriteField("voice_id", voiceID)
	}
	if groupID != "" {
		_ = writer.WriteField("group_id", groupID)
	}
	if err = writer.Close(); err != nil {
		return nil, fmt.Errorf("构建表单失败: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, &body)
	if err != nil {
		return nil, fmt.Errorf("创建请求失败: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	req.Header.Set("Accept", "application/json")
	if groupID != "" {
		req.Header.Set("Group-Id", groupID)
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
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(respBody)))
	}

	var parsed map[string]any
	if err := json.Unmarshal(respBody, &parsed); err != nil {
		return nil, fmt.Errorf("解析响应失败: %w", err)
	}

	resolvedVoiceID := pickVoiceID(parsed)
	if resolvedVoiceID == "" {
		return nil, fmt.Errorf("响应中未返回 voice_id: %s", strings.TrimSpace(string(respBody)))
	}
	return &minimaxVoiceCloneResult{
		VoiceID:      resolvedVoiceID,
		RawResponse:  parsed,
		RequestID:    getStringAny(parsed, "request_id", "trace_id"),
		ResponseCode: resp.StatusCode,
	}, nil
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
