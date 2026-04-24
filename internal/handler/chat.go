package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"openai-backend/internal/dto"
	"openai-backend/internal/repo"
	"openai-backend/internal/service"
	"openai-backend/internal/task"
	"openai-backend/internal/upstream"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

var chatSvc = service.NewChatService(
	repo.NewAIModelRepo(),
	repo.NewCompletionRepo(),
	upstream.NewClient(nil),
)

// ChatCompletions 兼容 OpenAI Chat Completions：支持 stream=true/false，并将每次请求落库以便后续 GET/DELETE/CANCEL。
func ChatCompletions(c *gin.Context) {
	var req dto.ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body: " + err.Error(),
				"type":    "invalid_request_error",
			},
		})
		return
	}

	ownedBy, err := chatSvc.GetEnabledModelOrErr(req.Model)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Model not available",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	completionID := "chatcmpl-" + uuid.NewString()
	reqBytes, _ := json.Marshal(req)
	chatSvc.CreateRunningCompletion(completionID, reqBytes)

	attempts := chatSvc.BuildAttempts(ownedBy)

	// 非流式：优先转发上游（支持 fallback），否则返回 mock
	if !req.Stream {
		if status, ct, body, ok := chatSvc.ProxyNonStream(c.Request.Context(), attempts, completionID, reqBytes); ok {
			if status == http.StatusOK {
				chatSvc.MarkCompleted(completionID, body)
			} else {
				chatSvc.MarkFailed(completionID, body)
			}
			c.Data(status, ct, body)
			return
		}

		resp := dto.ChatCompletionResponse{
			ID:      completionID,
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []dto.Choice{{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: "Hello! This is a mock response.",
				},
				FinishReason: "stop",
			}},
		}

		respBytes, _ := json.Marshal(resp)
		chatSvc.MarkCompleted(completionID, respBytes)
		c.JSON(http.StatusOK, resp)
		return
	}

	// 流式：优先上游 SSE（支持 fallback），否则走 mock
	ctx, cancel := context.WithCancel(c.Request.Context())
	task.Register(completionID, cancel)
	defer task.Finish(completionID)

	if streamRes, errStatus, errCT, errBody, ok := chatSvc.OpenUpstreamStream(ctx, attempts, completionID, reqBytes); ok {
		if streamRes == nil {
			c.Data(errStatus, errCT, errBody)
			chatSvc.MarkFailed(completionID, errBody)
			return
		}
		defer streamRes.Resp.Body.Close()

		c.Header("Content-Type", "text/event-stream")
		c.Header("Cache-Control", "no-cache")
		c.Header("Connection", "keep-alive")
		c.Status(http.StatusOK)

		flusher, ok := c.Writer.(http.Flusher)
		if !ok {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": gin.H{"message": "Streaming unsupported", "type": "server_error"},
			})
			return
		}

		reader := bufio.NewReader(streamRes.Resp.Body)
		var generated strings.Builder

		for {
			line, readErr := reader.ReadBytes('\n')
			if len(line) > 0 {
				if bytes.HasPrefix(line, []byte("data: ")) {
					payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data: ")))
					if bytes.Equal(payload, []byte("[DONE]")) {
						_, _ = c.Writer.Write(line)
						flusher.Flush()
						break
					}

					var obj map[string]any
					if json.Unmarshal(payload, &obj) == nil {
						obj["id"] = completionID
						if b, e := json.Marshal(obj); e == nil {
							line = append([]byte("data: "), append(b, '\n')...)
						}

						if choices, ok := obj["choices"].([]any); ok && len(choices) > 0 {
							if c0, ok := choices[0].(map[string]any); ok {
								if delta, ok := c0["delta"].(map[string]any); ok {
									if content, ok := delta["content"].(string); ok {
										generated.WriteString(content)
									}
								}
							}
						}
					}
				}

				_, _ = c.Writer.Write(line)
				flusher.Flush()
			}
			if readErr != nil {
				break
			}
		}

		finalResp := dto.ChatCompletionResponse{
			ID:      completionID,
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []dto.Choice{{
				Index: 0,
				Message: dto.Message{
					Role:    "assistant",
					Content: generated.String(),
				},
				FinishReason: "stop",
			}},
		}

		respBytes, _ := json.Marshal(finalResp)
		if ctx.Err() != nil {
			finalResp.Choices[0].FinishReason = "cancelled"
			respBytes, _ = json.Marshal(finalResp)
			now := time.Now()
			chatSvc.MarkCancelled(completionID, respBytes, &now)
			return
		}
		chatSvc.MarkCompleted(completionID, respBytes)
		return
	}

	// mock stream
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{"message": "Streaming unsupported", "type": "server_error"},
		})
		return
	}

	writeChunk := func(delta map[string]string) {
		payload := map[string]any{
			"id":      completionID,
			"object":  "chat.completion.chunk",
			"created": time.Now().Unix(),
			"model":   req.Model,
			"choices": []any{map[string]any{"index": 0, "delta": delta}},
		}
		b, _ := json.Marshal(payload)
		fmt.Fprintf(c.Writer, "data: %s\n\n", b)
		flusher.Flush()
	}

	writeChunk(map[string]string{"role": "assistant"})

	generated := ""
	parts := make([]string, 0, 24)
	parts = append(parts, "Hello! ")
	parts = append(parts, "This is a mock response. ")
	parts = append(parts, fmt.Sprintf("model=%s temperature=%v messages=%d ", req.Model, req.Temperature, len(req.Messages)))
	for i := 0; i < 20; i++ {
		parts = append(parts, fmt.Sprintf("chunk-%02d ", i+1))
	}

	for _, part := range parts {
		select {
		case <-ctx.Done():
			finalResp := dto.ChatCompletionResponse{
				ID:      completionID,
				Object:  "chat.completion",
				Created: time.Now().Unix(),
				Model:   req.Model,
				Choices: []dto.Choice{{
					Index:        0,
					Message:      dto.Message{Role: "assistant", Content: generated},
					FinishReason: "cancelled",
				}},
			}
			respBytes, _ := json.Marshal(finalResp)
			now := time.Now()
			chatSvc.MarkCancelled(completionID, respBytes, &now)

			fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
			flusher.Flush()
			return
		default:
		}

		time.Sleep(1 * time.Second)
		generated += part
		writeChunk(map[string]string{"content": part})
	}

	finalResp := dto.ChatCompletionResponse{
		ID:      completionID,
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   req.Model,
		Choices: []dto.Choice{{
			Index:        0,
			Message:      dto.Message{Role: "assistant", Content: generated},
			FinishReason: "stop",
		}},
	}
	respBytes, _ := json.Marshal(finalResp)
	chatSvc.MarkCompleted(completionID, respBytes)

	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	flusher.Flush()
}

// GetCompletion 通过 completion_id 查询生成结果
func GetCompletion(c *gin.Context) {
	id := c.Param("id")
	respBytes, err := chatSvc.GetStoredResponse(id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Not found", "type": "not_found"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to get", "type": "internal_server_error"}})
		return
	}
	c.Data(http.StatusOK, "application/json", respBytes)
}

// DeleteCompletion 通过 completion_id 删除生成结果
func DeleteCompletion(c *gin.Context) {
	id := c.Param("id")
	if err := chatSvc.DeleteStoredCompletion(id); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Not found", "type": "not_found"}})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to delete", "type": "internal_server_error"}})
		return
	}
	c.JSON(http.StatusOK, dto.DeleteCompletionResponse{ID: id, Object: "chat.completion.deleted", Deleted: true})
}

// CancelCompletion 取消正在进行的生成（仅对 stream=true 的任务有效）
func CancelCompletion(c *gin.Context) {
	id := c.Param("id")
	if ok := task.Cancel(id); !ok {
		c.JSON(http.StatusNotFound, gin.H{"error": gin.H{"message": "Not found", "type": "not_found"}})
		return
	}

	c.JSON(http.StatusOK, dto.CancelCompletionResponse{ID: id, Object: "chat.completion.cancelled", Cancelled: true})
}
