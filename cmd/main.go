package main

import (
	"openai-backend/internal/middleware"
	"openai-backend/internal/model"

	"github.com/gin-gonic/gin"
)

func main() {
	model.InitDB()
	r := gin.Default()
	r.Use(middleware.AuthMiddleware())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.Run(":8080")
}
