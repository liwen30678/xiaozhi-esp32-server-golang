package controllers

import (
	"xiaozhi/manager/backend/models"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// initUserVoiceCloneQuotas 为用户初始化每个TTS配置的复刻额度。
// maxCount=0 表示默认禁止使用，需管理员后续分配。
func initUserVoiceCloneQuotas(db *gorm.DB, userID uint, maxCount int) error {
	var ttsConfigIDs []string
	if err := db.Model(&models.Config{}).
		Where("type = ?", "tts").
		Pluck("config_id", &ttsConfigIDs).Error; err != nil {
		return err
	}
	if len(ttsConfigIDs) == 0 {
		return nil
	}

	quotas := make([]models.UserVoiceCloneQuota, 0, len(ttsConfigIDs))
	for _, configID := range ttsConfigIDs {
		quotas = append(quotas, models.UserVoiceCloneQuota{
			UserID:      userID,
			TTSConfigID: configID,
			MaxCount:    maxCount,
			UsedCount:   0,
		})
	}

	return db.Clauses(clause.OnConflict{DoNothing: true}).Create(&quotas).Error
}
