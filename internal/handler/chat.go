package handler

import (
	"encoding/json"
	"net/http"
	"openai-backend/internal/model"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type ChatCompletionRequest struct {
	Model       string    `json:"model"`
	Messages    []Message `json:"messages"`
	Stream      bool      `json:"stream"`
	Temperature float64   `json:"temperature"`
}

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ChatCompletionResponse struct {
	ID      string   `json:"id"`
	Object  string   `json:"object"`
	Created int64    `json:"created"`
	Model   string   `json:"model"`
	Choices []Choice `json:"choices"`
}

type Choice struct {
	Index        int     `json:"index"`
	Message      Message `json:"message"`
	FinishReason string  `json:"finish_reason"`
}

type DeleteCompletionResponse struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Deleted bool   `json:"deleted"`
}

func ChatCompletions(c *gin.Context) {
	var req ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	completionID := "chatcmpl-" + uuid.NewString()

	resp := ChatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []Choice{
			{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: "Hello! This is a mock response.",
				},
				FinishReason: "stop",
			},
		},
	}

	reqBytes, _ := json.Marshal(req)
	respBytes, _ := json.Marshal(resp)

	_ = model.DB.Create(&model.Completion{
		CompletionID: completionID,
		Request:      string(reqBytes),
		Response:     string(respBytes),
	}).Error
	c.JSON(http.StatusOK, resp)
}

// GetCompletion 通过 completion_id 查询生成结果
func GetCompletion(c *gin.Context) {
	id := c.Param("id")
	var completion model.Completion
	if err := model.DB.Where("completion_id = ?", id).First(&completion).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Not found", "type": "not_found"}})
		return
	}
	// 直接返回存储的响应内容
	c.Data(http.StatusOK, "application/json", []byte(completion.Response))
}

// DeleteCompletion 通过 completion_id 删除生成结果
func DeleteCompletion(c *gin.Context) {
	id := c.Param("id")
	var completion model.Completion
	if err := model.DB.Where("completion_id = ?", id).First(&completion).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Not found", "type": "not_found"}})
		return
	}
	if err := model.DB.Delete(&completion).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to delete", "type": "internal_server_error"}})
		return
	}
	c.JSON(http.StatusOK, DeleteCompletionResponse{
		ID:      id,
		Object:  "chat.completion",
		Deleted: true,
	})
}
