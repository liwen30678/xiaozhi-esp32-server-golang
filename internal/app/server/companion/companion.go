package companion

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	log "xiaozhi-esp32-server-golang/logger"
)

// Config 伴侣模块配置
type Config struct {
	Enable          bool   `mapstructure:"enable"`            // 是否启用
	ImageAPIBaseURL string `mapstructure:"image_api_base_url"` // 生图API地址
	ImageAPIKey     string `mapstructure:"image_api_key"`      // 生图API Key
	ImageModel      string `mapstructure:"image_model"`       // 生图模型名
	ImageAPIAuthKey string `mapstructure:"image_api_auth_key"` // ESP下载图片时的鉴权Key
}

// DefaultConfig 默认配置
var DefaultConfig = Config{
	ImageAPIBaseURL: "https://router.ryanli.cn/v1",
	ImageModel:      "grokimage",
}

// Companion 伴侣模块
type Companion struct {
	config Config
	client *http.Client
}

// New 创建伴侣模块实例
func New(cfg Config) *Companion {
	return &Companion{
		config: cfg,
		client: &http.Client{Timeout: 120 * time.Second},
	}
}

// GenerateImageResponse 生图API响应
type GenerateImageResponse struct {
	Created int64 `json:"created"`
	Data    []struct {
		URL           string `json:"url"`
		B64JSON       string `json:"b64_json,omitempty"`
		RevisedPrompt string `json:"revised_prompt,omitempty"`
	} `json:"data"`
}

// ImagePushMessage 推送给ESP的图片消息
type ImagePushMessage struct {
	Type     string `json:"type"`
	Action   string `json:"action"`
	ImageURL string `json:"image_url"`
	AuthKey  string `json:"auth_key,omitempty"`
}

// GenerateAndPush 生成图片并推送消息（调用方负责实际推送到设备）
func (c *Companion) GenerateAndPush(ctx context.Context, prompt string) (*ImagePushMessage, error) {
	// 1. 调用生图API
	imgResp, err := c.generateImage(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("生图失败: %w", err)
	}

	if len(imgResp.Data) == 0 {
		return nil, fmt.Errorf("生图API返回空数据")
	}

	imageURL := imgResp.Data[0].URL
	if imageURL == "" {
		return nil, fmt.Errorf("生图API返回的URL为空")
	}

	log.Infof("[Companion] 生图成功, URL: %s", imageURL)

	// 2. 构造推送消息
	msg := &ImagePushMessage{
		Type:     "companion",
		Action:   "display_image",
		ImageURL: imageURL,
		AuthKey:  c.config.ImageAPIAuthKey,
	}

	return msg, nil
}

// generateImage 调用生图API
func (c *Companion) generateImage(ctx context.Context, prompt string) (*GenerateImageResponse, error) {
	reqBody := map[string]interface{}{
		"model":     c.config.ImageModel,
		"prompt":    prompt,
		"max_tokens": 1000,
	}

	jsonBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("marshal request error: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.config.ImageAPIBaseURL, "/")+"/images/generations",
		bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("create request error: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.config.ImageAPIKey)

	resp, err := c.client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request error: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response error: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("生图API返回错误 %d: %s", resp.StatusCode, string(body))
	}

	var result GenerateImageResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response error: %w", err)
	}

	return &result, nil
}
