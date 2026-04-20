package handler

import (
	"context"
	"encoding/json"
	"fmt"
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

type CancelCompletionResponse struct {
	ID        string `json:"id"`
	Object    string `json:"object"`
	Cancelled bool   `json:"cancelled"`
}

func ChatCompletions(c *gin.Context) {
	var req ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	completionID := "chatcmpl-" + uuid.NewString()
	reqBytes, _ := json.Marshal(req)

	_ = model.DB.Create(&model.Completion{
		CompletionID: completionID,
		Request:      string(reqBytes),
		Response:     "",
		Status:       "running",
	}).Error

	if req.Stream {
		ctx, cancel := context.WithCancel(c.Request.Context())
		RegisterTask(completionID, cancel)
		defer FinishTask(completionID)

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{
					"message": "Streaming unsupported",
					"type":    "server_error",
				},
			})
			return
		}

		writeChunk := func(delta map[string]string) {
			payload := map[string]any{
				"id":      completionID,
				"object":  "chat.completion.chunk",
				"created": time.Now().Unix(),
				"model":   req.Model,
				"choices": []any{
					map[string]any{
						"index": 0,
						"delta": delta,
					},
				},
			}
			b, _ := json.Marshal(payload)
			fmt.Fprintf(c.Writer, "data: %s\n\n", b)
			flusher.Flush()
		}

		// 先发 role
		writeChunk(map[string]string{"role": "assistant"})

		generated := ""
		parts := make([]string, 0, 24)
		parts = append(parts, "Hello! ")
		parts = append(parts, "This is a mock response. ")
		parts = append(parts, fmt.Sprintf("model=%s temperature=%v messages=%d ", req.Model, req.Temperature, len(req.Messages)))
		// 为了便于测试 cancel，这里故意拉长流式输出（约 20 秒，每秒一个 chunk）
		for i := 0; i < 20; i++ {
			parts = append(parts, fmt.Sprintf("chunk-%02d ", i+1))
		}

		for _, part := range parts {
			select {
			case <-ctx.Done():
				// 持久化已生成的内容，方便后续 GET 查询
				finalResp := ChatCompletionResponse{
					ID:      completionID,
					Object:  "chat.completion",
					Created: time.Now().Unix(),
					Model:   req.Model,
					Choices: []Choice{{
						Index: 0,
						Message: Message{
							Role:    "assistant",
							Content: generated,
						},
						FinishReason: "cancelled",
					}},
				}
				respBytes, _ := json.Marshal(finalResp)
				now := time.Now()
				_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
					"response":      string(respBytes),
					"status":        "cancelled",
					"cancelled_at":  &now,
				}).Error

				fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
				flusher.Flush()
				return
			default:
			}

			time.Sleep(1 * time.Second)
			generated += part
			writeChunk(map[string]string{"content": part})
		}

		finalResp := ChatCompletionResponse{
			ID:      completionID,
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []Choice{{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: generated,
				},
				FinishReason: "stop",
			}},
		}
		respBytes, _ := json.Marshal(finalResp)
		_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
			"response": string(respBytes),
			"status":   "completed",
		}).Error

		fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

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

	respBytes, _ := json.Marshal(resp)
	_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
		"response": string(respBytes),
		"status":   "completed",
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
		Object:  "chat.completion.deleted",
		Deleted: true,
	})
}

// CancelCompletion 取消正在进行的生成（仅对 stream=true 的任务有效）
func CancelCompletion(c *gin.Context) {
	id := c.Param("id")
	if ok := CancelTask(id); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Not found", "type": "not_found"}})
		return
	}

	c.JSON(http.StatusOK, CancelCompletionResponse{
		ID:        id,
		Object:    "chat.completion.cancelled",
		Cancelled: true,
	})
}
