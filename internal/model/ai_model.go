package model

import "time"

type AIModel struct {
	ID      uint      `gorm:"primaryKey"`
	ModelID string    `gorm:"column:model_id;size:64;uniqueIndex; not null" json:"model_id"`
	OwnedBy string    `gorm:"column:owned_by;size:64; not null" json:"owned_by"`
	Enabled bool      `gorm:"column:enabled; not null; default:true" json:"enabled"`
	Created time.Time `gorm:"column:created; not null" json:"created"`
}
