package main

import (
	"openai-backend/internal/handler"
	"openai-backend/internal/middleware"
	"openai-backend/internal/model"

	"github.com/gin-gonic/gin"
)

func main() {
	model.InitDB()
	r := gin.Default()
	_ = r.SetTrustedProxies([]string{"127.0.0.1", "::1"})
	r.Use(middleware.AuthMiddleware())
	r.GET("/healthz", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})
	r.POST("/v1/chat/completions", handler.ChatCompletions)
	r.GET("/v1/chat/completions/:id", handler.GetCompletion)
	r.DELETE("/v1/chat/completions/:id", handler.DeleteCompletion)
	r.POST("/v1/chat/completions/:id/cancel", handler.CancelCompletion)
	r.GET("/v1/models", handler.ListModels)
	r.POST("/v1/models", handler.ListModels)
	r.Run(":8091")
}
