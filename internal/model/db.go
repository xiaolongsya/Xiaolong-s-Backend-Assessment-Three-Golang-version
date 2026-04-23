package model

import (
	"log"
	"os"
	"strings"

	"gorm.io/driver/mysql"
	"gorm.io/gorm"
)

var DB *gorm.DB

// InitDB 初始化 MySQL 连接并执行数据库迁移（Completion/AIModel）。
func InitDB() {
	dsn := strings.TrimSpace(os.Getenv("MYSQL_DSN"))
	if dsn == "" {
		log.Fatal("MYSQL_DSN environment variable is required")
	}
	var err error
	DB, err = gorm.Open(mysql.Open(dsn), &gorm.Config{})
	if err != nil {
		log.Fatalf("Failed to connect to database: %v", err)
	}
	if err := DB.AutoMigrate(&Completion{}, &AIModel{}, &File{}); err != nil {
		log.Fatalf("Failed to migrate database: %v", err)
	}
}
