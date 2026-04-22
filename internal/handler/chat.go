package handler

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"hash/fnv"
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

type upstreamConfig struct {
	baseURL string
	apiKey  string
}

func envKeySuffixFromOwnedBy(ownedBy string) string {
	s := strings.TrimSpace(ownedBy)
	if s == "" {
		return ""
	}
	s = strings.ToUpper(s)
	var b strings.Builder
	b.Grow(len(s))
	lastUnderscore := false
	for _, r := range s {
		isAZ := r >= 'A' && r <= 'Z'
		is09 := r >= '0' && r <= '9'
		if isAZ || is09 {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	out := strings.Trim(b.String(), "_")
	return out
}

func parseCommaList(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		v := strings.TrimSpace(p)
		if v != "" {
			out = append(out, v)
		}
	}
	return out
}

func pickKeyDeterministic(keys []string, seed string) string {
	if len(keys) == 0 {
		return ""
	}
	if len(keys) == 1 {
		return keys[0]
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	idx := int(h.Sum32() % uint32(len(keys)))
	return keys[idx]
}

func shouldFallbackForStatus(statusCode int) bool {
	if statusCode >= 500 {
		return true
	}
	switch statusCode {
	case http.StatusTooManyRequests, http.StatusUnauthorized, http.StatusForbidden:
		return true
	default:
		return false
	}
}

// resolveUpstreamForModel 按模型所属 provider（ai_models.owned_by）解析上游配置。
//
// 优先级（高 → 低）：
// - UPSTREAM_<PROVIDER>_BASE_URL
// - UPSTREAM_BASE_URL
//
// - UPSTREAM_<PROVIDER>_API_KEYS（逗号分隔，多 key）
// - UPSTREAM_<PROVIDER>_API_KEY（单 key）
// - UPSTREAM_API_KEY（单 key）
func resolveUpstreamForModel(m model.AIModel, completionID string) upstreamConfig {
	provider := envKeySuffixFromOwnedBy(m.OwnedBy)
	return resolveUpstreamForProvider(provider, completionID)
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

	primary := envKeySuffixFromOwnedBy(m.OwnedBy)
	fallbackMap := parseUpstreamFallbacks()

	attempts := make([]string, 0, 3)
	if primary != "" {
		attempts = append(attempts, primary)
		attempts = append(attempts, fallbackMap[primary]...)
	}

	// 如果配置了上游，并且是非流式，则转发到上游（OpenAI 兼容，支持按 attempts 自动回退）
	if !req.Stream && len(attempts) > 0 {
		bodyBytes, err := json.Marshal(req)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to marshal request", "type": "server_error"}})
			return
		}

		triedAny := false
		var lastStatus int
		var lastCT string
		var lastRespBytes []byte
		var lastErr error

		for _, provider := range attempts {
			up := resolveUpstreamForProvider(provider, completionID)
			if up.baseURL == "" || up.apiKey == "" {
				continue
			}
			triedAny = true

			upstreamURL := up.baseURL + "/chat/completions"
			httpReq, err := http.NewRequestWithContext(c.Request.Context(), http.MethodPost, upstreamURL, bytes.NewReader(bodyBytes))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to create upstream request", "type": "server_error"}})
				return
			}
			httpReq.Header.Set("Content-Type", "application/json")
			httpReq.Header.Set("Authorization", "Bearer "+up.apiKey)

			resp, err := (&http.Client{}).Do(httpReq)
			if err != nil {
				lastErr = err
				lastStatus = http.StatusBadGateway
				lastCT = "application/json"
				lastRespBytes = nil
				continue
			}

			respBytes, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			ct := resp.Header.Get("Content-Type")
			if ct == "" {
				ct = "application/json"
			}

			if resp.StatusCode == http.StatusOK {
				var obj map[string]any
				if err := json.Unmarshal(respBytes, &obj); err == nil {
					obj["id"] = completionID
					if b, err := json.Marshal(obj); err == nil {
						respBytes = b
					}
				}

				_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
					"response": string(respBytes),
					"status":   "completed",
				}).Error

				c.Data(resp.StatusCode, ct, respBytes)
				return
			}

			// 非 200：记录最后一次响应；判断是否回退
			lastStatus = resp.StatusCode
			lastCT = ct
			lastRespBytes = respBytes
			lastErr = nil

			if shouldFallbackForStatus(resp.StatusCode) {
				continue
			}

			// 非回退类错误，直接透传并落库 failed
			_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
				"response": string(respBytes),
				"status":   "failed",
			}).Error
			c.Data(resp.StatusCode, ct, respBytes)
			return
		}

		// 如果至少尝试过一个上游，但都失败了：返回最后一次可用的错误
		if triedAny {
			status := lastStatus
			if status == 0 {
				status = http.StatusBadGateway
			}
			ct := lastCT
			if ct == "" {
				ct = "application/json"
			}

			respBytes := lastRespBytes
			if respBytes == nil {
				msg := "Upstream request failed"
				if lastErr != nil {
					msg = msg + ": " + lastErr.Error()
				}
				respBytes, _ = json.Marshal(map[string]any{
					"error": map[string]any{
						"message": msg,
						"type":    "upstream_error",
					},
				})
			}

			_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
				"response": string(respBytes),
				"status":   "failed",
			}).Error
			c.Data(status, ct, respBytes)
			return
		}
	}
	// 如果配置了上游，并且是流式，则转发上游 SSE（并把 id 改为本服务的 completionID）
	if req.Stream && len(attempts) > 0 {
		ctx, cancel := context.WithCancel(c.Request.Context())
		RegisterTask(completionID, cancel)
		defer FinishTask(completionID)

		bodyBytes, err := json.Marshal(req) // req.Stream = true
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to marshal request", "type": "server_error"}})
			return
		}

		triedAny := false
		var lastStatus int
		var lastCT string
		var lastRespBytes []byte
		var lastErr error

		for _, provider := range attempts {
			up := resolveUpstreamForProvider(provider, completionID)
			if up.baseURL == "" || up.apiKey == "" {
				continue
			}
			triedAny = true

			upstreamURL := up.baseURL + "/chat/completions"
			httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, upstreamURL, bytes.NewReader(bodyBytes))
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{"error": gin.H{"message": "Failed to create upstream request", "type": "server_error"}})
				return
			}
			httpReq.Header.Set("Content-Type", "application/json")
			httpReq.Header.Set("Authorization", "Bearer "+up.apiKey)

			resp, err := (&http.Client{}).Do(httpReq) // 不要设置 Timeout，流式会一直跑
			if err != nil {
				lastErr = err
				lastStatus = http.StatusBadGateway
				lastCT = "application/json"
				lastRespBytes = nil
				continue
			}

			// 非 200：读完错误体后决定是否回退
			if resp.StatusCode != http.StatusOK {
				respBytes, _ := io.ReadAll(resp.Body)
				_ = resp.Body.Close()
				ct := resp.Header.Get("Content-Type")
				if ct == "" {
					ct = "application/json"
				}

				lastStatus = resp.StatusCode
				lastCT = ct
				lastRespBytes = respBytes
				lastErr = nil

				if shouldFallbackForStatus(resp.StatusCode) {
					continue
				}

				c.Data(resp.StatusCode, ct, respBytes)
				_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
					"response": string(respBytes),
					"status":   "failed",
				}).Error
				return
			}

			// 成功选中该 provider，进入流式转发
			defer resp.Body.Close()

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

		// 所有上游都在开始流式前失败：仅当确实尝试过上游时返回错误，否则继续走 mock
		if triedAny {
			status := lastStatus
			if status == 0 {
				status = http.StatusBadGateway
			}
			ct := lastCT
			if ct == "" {
				ct = "application/json"
			}

			respBytes := lastRespBytes
			if respBytes == nil {
				msg := "Upstream request failed"
				if lastErr != nil {
					msg = msg + ": " + lastErr.Error()
				}
				respBytes, _ = json.Marshal(map[string]any{
					"error": map[string]any{
						"message": msg,
						"type":    "upstream_error",
					},
				})
			}

			c.Data(status, ct, respBytes)
			_ = model.DB.Model(&model.Completion{}).Where("completion_id = ?", completionID).Updates(map[string]any{
				"response": string(respBytes),
				"status":   "failed",
			}).Error
			return
		}
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

func parseUpstreamFallbacks() map[string][]string {
	raw := strings.TrimSpace(os.Getenv("UPSTREAM_FALLBACKS"))
	if raw == "" {
		return map[string][]string{}
	}

	out := make(map[string][]string)
	pairs := strings.Split(raw, ";")
	for _, pair := range pairs {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}

		kv := strings.SplitN(pair, "=", 2)
		if len(kv) != 2 {
			continue
		}

		primary := envKeySuffixFromOwnedBy(kv[0]) // 关键：统一成 VOLCANO / MINIMAX 这种 suffix
		if primary == "" {
			continue
		}

		rawList := strings.TrimSpace(kv[1])
		if rawList == "" {
			continue
		}

		items := strings.Split(rawList, ",")
		var fallbacks []string
		for _, it := range items {
			s := envKeySuffixFromOwnedBy(it)
			if s != "" && s != primary {
				fallbacks = append(fallbacks, s)
			}
		}
		if len(fallbacks) > 0 {
			out[primary] = fallbacks
		}
	}

	return out
}

func resolveUpstreamForProvider(providerSuffix string, completionID string) upstreamConfig {
	provider := strings.TrimSpace(providerSuffix)

	baseURL := ""
	if provider != "" {
		baseURL = strings.TrimSpace(os.Getenv("UPSTREAM_" + provider + "_BASE_URL"))
	}
	if baseURL == "" {
		baseURL = strings.TrimSpace(os.Getenv("UPSTREAM_BASE_URL"))
	}
	baseURL = strings.TrimRight(baseURL, "/")

	var keys []string
	if provider != "" {
		keys = parseCommaList(os.Getenv("UPSTREAM_" + provider + "_API_KEYS"))
		if len(keys) == 0 {
			k := strings.TrimSpace(os.Getenv("UPSTREAM_" + provider + "_API_KEY"))
			if k != "" {
				keys = []string{k}
			}
		}
	}
	if len(keys) == 0 {
		k := strings.TrimSpace(os.Getenv("UPSTREAM_API_KEY"))
		if k != "" {
			keys = []string{k}
		}
	}

	apiKey := pickKeyDeterministic(keys, completionID)
	return upstreamConfig{baseURL: baseURL, apiKey: apiKey}
}
