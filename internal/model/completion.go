package model

import (
	"gorm.io/gorm"
)

// Completion 表结构，存储每次生成请求和响应
type Completion struct {
	gorm.Model
	CompletionID string `gorm:"uniqueIndex"` // 唯一ID
	Request      string // 原始请求内容（JSON字符串）
	Response     string // 生成的响应内容（JSON字符串）
}
