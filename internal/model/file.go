package model

import "gorm.io/gorm"

type File struct {
	gorm.Model
	FileID      string `gorm:"size:64;uniqueIndex"`
	Bytes       int64  `gorm:"not null"`
	Filename    string `gorm:"size:255;not null"`
	Purpose     string `gorm:"size:64;index"`
	MimeType    string `gorm:"size:128"`
	StoragePath string `gorm:"size:512;not null"`
}
