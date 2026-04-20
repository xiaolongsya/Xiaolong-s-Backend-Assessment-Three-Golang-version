package model

import (
	"time"

	"gorm.io/gorm"
)

// Completion 表结构，存储每次生成请求和响应
type Completion struct {
	gorm.Model
	CompletionID string `gorm:"size:64;uniqueIndex"`
	Request      string `gorm:"type:longtext"`
	Response     string `gorm:"type:longtext"`
	Status       string `gorm:"size:16;index"`
	CancelledAt  *time.Time
}
