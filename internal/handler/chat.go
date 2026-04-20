package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"openai-backend/internal/model"
	"os"
	"strings"
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

// ChatCompletions 兼容 OpenAI Chat Completions：支持 stream=true/false，并将每次请求落库以便后续 GET/DELETE/CANCEL。
func ChatCompletions(c *gin.Context) {
	var req ChatCompletionRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	var m model.AIModel
	if err := model.DB.
		Where("model_id = ? AND enabled = ?", req.Model, true).
		First(&m).Error; err != nil {
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

	_ = model.DB.Create(&model.Completion{
		CompletionID: completionID,
		Request:      string(reqBytes),
		Response:     "",
		Status:       "running",
	}).Error

	upstreamBase := strings.TrimRight(strings.TrimSpace(os.Getenv("UPSTREAM_BASE_URL")), "/")
	upstreamKey := strings.TrimSpace(os.Getenv("UPSTREAM_API_KEY"))

	// 如果配置了上游，并且是非流式，则转发到 MiniMax（OpenAI 兼容）
	if upstreamBase != "" && upstreamKey != "" && !req.Stream {
		upstreamURL := upstreamBase + "/chat/completions"

		bodyBytes, err := json.Marshal(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to marshal request", "type": "server_error"}})
			return
		}

		httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstreamURL, bytes.NewReader(bodyBytes))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to create upstream request", "type": "server_error"}})
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+upstreamKey)

		resp, err := (&http.Client{}).Do(httpReq)
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "Upstream request failed: " + err.Error(), "type": "upstream_error"}})
			_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
				"status": "failed",
			}).Error
			return
		}
		defer resp.Body.Close()

		respBytes, _ := io.ReadAll(resp.Body)

		if resp.StatusCode == http.StatusOK {
			var obj map[string]any
			if err := json.Unmarshal(respBytes, &obj); err == nil {
				obj["id"] = completionID
				if b, err := json.Marshal(obj); err == nil {
					respBytes = b
				}
			}
		}

		// 持久化上游响应
		_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
			"response": string(respBytes),
			"status":   "completed",
		}).Error

		// 透传上游响应（包含非 200 的错误体）
		ct := resp.Header.Get("Content-Type")
		if ct == "" {
			ct = "application/json"
		}
		c.Data(resp.StatusCode, ct, respBytes)
		return
	}
	// 如果配置了上游，并且是流式，则转发上游 SSE（并把 id 改为本服务的 completionID）
	if upstreamBase != "" && upstreamKey != "" && req.Stream {
		ctx, cancel := context.WithCancel(c.Request.Context())
		RegisterTask(completionID, cancel)
		defer FinishTask(completionID)

		upstreamURL := upstreamBase + "/chat/completions"
		bodyBytes, err := json.Marshal(req) // req.Stream = true
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to marshal request", "type": "server_error"}})
			return
		}

		httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(bodyBytes))
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to create upstream request", "type": "server_error"}})
			return
		}
		httpReq.Header.Set("Content-Type", "application/json")
		httpReq.Header.Set("Authorization", "Bearer "+upstreamKey)

		resp, err := (&http.Client{}).Do(httpReq) // 不要设置 Timeout，流式会一直跑
		if err != nil {
			c.JSON(http.StatusBadGateway, gin.H{"error": gin.H{"message": "Upstream request failed: " + err.Error(), "type": "upstream_error"}})
			_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
				"status": "failed",
			}).Error
			return
		}
		defer resp.Body.Close()

		// 透传上游状态码（非 200 直接返回错误体）
		if resp.StatusCode != http.StatusOK {
			respBytes, _ := io.ReadAll(resp.Body)
			ct := resp.Header.Get("Content-Type")
			if ct == "" {
				ct = "application/json"
			}
			c.Data(resp.StatusCode, ct, respBytes)
			_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
				"response": string(respBytes),
				"status":   "failed",
			}).Error
			return
		}

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

		reader := bufio.NewReader(resp.Body)
		var generated strings.Builder

		for {
			line, err := reader.ReadBytes('\n')
			if len(line) > 0 {
				// 只处理 data: 行，替换其中 JSON 的 id，并尽量累积 content 方便落库
				if bytes.HasPrefix(line, []byte("data: ")) {
					payload := bytes.TrimSpace(bytes.TrimPrefix(line, []byte("data: ")))

					if bytes.Equal(payload, []byte("[DONE]")) {
						// 原样写回 DONE
						_, _ = c.Writer.Write(line)
						flusher.Flush()
						break
					}

					// 尝试把 data: 后面的 JSON 解析出来，替换 id
					var obj map[string]any
					if json.Unmarshal(payload, &obj) == nil {
						obj["id"] = completionID
						if b, e := json.Marshal(obj); e == nil {
							// 回写成 data: <new-json>\n
							line = append([]byte("data: "), append(b, '\n')...)
						}

						// 尝试抽取 delta.content（尽力而为，不影响主流程）
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
			if err != nil {
				// 上游断开（包括 cancel 导致的 ctx cancel）
				break
			}
		}

		// cancel 或完成后，落库一个最终 JSON，保证 GET /:id 能返回 JSON
		finalResp := ChatCompletionResponse{
			ID:      completionID,
			Object:  "chat.completion",
			Created: time.Now().Unix(),
			Model:   req.Model,
			Choices: []Choice{{
				Index: 0,
				Message: Message{
					Role:    "assistant",
					Content: generated.String(),
				},
				FinishReason: "stop",
			}},
		}

		// 如果是被取消（ctx.Done），标记 cancelled
		if ctx.Err() != nil {
			finalResp.Choices[0].FinishReason = "cancelled"
			respBytes, _ := json.Marshal(finalResp)
			now := time.Now()
			_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
				"response":     string(respBytes),
				"status":       "cancelled",
				"cancelled_at": &now,
			}).Error
			return
		}

		respBytes, _ := json.Marshal(finalResp)
		_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
			"response": string(respBytes),
			"status":   "completed",
		}).Error
		return
	}

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
					"response":     string(respBytes),
					"status":       "cancelled",
					"cancelled_at": &now,
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
