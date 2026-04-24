package service

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"openai-backend/internal/repo"
	"openai-backend/internal/upstream"
	"strings"
	"time"

	"gorm.io/gorm"
)

type ChatService struct {
	models      *repo.AIModelRepo
	completions *repo.CompletionRepo
	upstream    *upstream.Client
}

func NewChatService(models *repo.AIModelRepo, completions *repo.CompletionRepo, upstreamClient *upstream.Client) *ChatService {
	return &ChatService{
		models:      models,
		completions: completions,
		upstream:    upstreamClient,
	}
}

func (s *ChatService) GetEnabledModelOrErr(modelID string) (ownedBy string, err error) {
	m, err := s.models.GetEnabledByModelID(modelID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", gorm.ErrRecordNotFound
		}
		return "", err
	}
	return m.OwnedBy, nil
}

func (s *ChatService) CreateRunningCompletion(completionID string, reqBytes []byte) {
	_ = s.completions.CreateRunning(completionID, reqBytes)
}

func (s *ChatService) MarkCompleted(completionID string, respBytes []byte) {
	_ = s.completions.UpdateFields(completionID, map[string]any{
		"response": string(respBytes),
		"status":   "completed",
	})
}

func (s *ChatService) MarkFailed(completionID string, respBytes []byte) {
	_ = s.completions.UpdateFields(completionID, map[string]any{
		"response": string(respBytes),
		"status":   "failed",
	})
}

func (s *ChatService) MarkCancelled(completionID string, respBytes []byte, cancelledAt *time.Time) {
	_ = s.completions.UpdateFields(completionID, map[string]any{
		"response":     string(respBytes),
		"status":       "cancelled",
		"cancelled_at": cancelledAt,
	})
}

func (s *ChatService) GetStoredResponse(completionID string) ([]byte, error) {
	row, err := s.completions.GetByCompletionID(completionID)
	if err != nil {
		return nil, err
	}
	return []byte(row.Response), nil
}

func (s *ChatService) DeleteStoredCompletion(completionID string) error {
	return s.completions.DeleteByCompletionID(completionID)
}

func (s *ChatService) BuildAttempts(ownedBy string) []string {
	primary := EnvKeySuffixFromOwnedBy(ownedBy)
	fallbackMap := ParseUpstreamFallbacksFromEnv()

	attempts := make([]string, 0, 3)
	if primary != "" {
		attempts = append(attempts, primary)
		attempts = append(attempts, fallbackMap[primary]...)
	}
	return attempts
}

func contentTypeOrJSON(ct string) string {
	ct = strings.TrimSpace(ct)
	if ct == "" {
		return "application/json"
	}
	return ct
}

// ProxyNonStream tries upstream providers in order and returns the final response.
// It rewrites upstream successful (200) JSON response field "id" to completionID.
func (s *ChatService) ProxyNonStream(ctx context.Context, attempts []string, completionID string, reqBody []byte) (status int, contentType string, respBody []byte, ok bool) {
	if len(attempts) == 0 {
		return 0, "", nil, false
	}

	var lastStatus int
	var lastCT string
	var lastBody []byte
	var lastErr error
	triedAny := false

	for _, provider := range attempts {
		cfg := ResolveUpstreamForProvider(provider, completionID)
		if cfg.BaseURL == "" || cfg.APIKey == "" {
			continue
		}
		triedAny = true
		url := cfg.BaseURL + "/chat/completions"

		resp, body, err := s.upstream.DoJSON(ctx, http.MethodPost, url, cfg.APIKey, reqBody)
		if err != nil {
			lastErr = err
			lastStatus = http.StatusBadGateway
			lastCT = "application/json"
			lastBody = nil
			continue
		}

		ct := contentTypeOrJSON(resp.Header.Get("Content-Type"))
		if resp.StatusCode == http.StatusOK {
			var obj map[string]any
			if json.Unmarshal(body, &obj) == nil {
				obj["id"] = completionID
				if b, err := json.Marshal(obj); err == nil {
					body = b
				}
			}
			return resp.StatusCode, ct, body, true
		}

		lastStatus = resp.StatusCode
		lastCT = ct
		lastBody = body
		lastErr = nil

		if ShouldFallbackForStatus(resp.StatusCode) {
			continue
		}
		return resp.StatusCode, ct, body, true
	}

	if !triedAny {
		return 0, "", nil, false
	}

	if lastBody == nil {
		msg := "Upstream request failed"
		if lastErr != nil {
			msg += ": " + lastErr.Error()
		}
		b, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": msg,
				"type":    "upstream_error",
			},
		})
		lastBody = b
		lastCT = "application/json"
		if lastStatus == 0 {
			lastStatus = http.StatusBadGateway
		}
	}

	return lastStatus, contentTypeOrJSON(lastCT), lastBody, true
}

type StreamResult struct {
	Resp *http.Response
	CT   string
}

// OpenUpstreamStream tries providers in order and returns the first 200 OK response.
// When upstream returns non-200 and is non-fallbackable, it returns status/body to be sent to client.
func (s *ChatService) OpenUpstreamStream(ctx context.Context, attempts []string, completionID string, reqBody []byte) (stream *StreamResult, errStatus int, errCT string, errBody []byte, ok bool) {
	if len(attempts) == 0 {
		return nil, 0, "", nil, false
	}

	var lastStatus int
	var lastCT string
	var lastBody []byte
	var lastErr error
	triedAny := false

	for _, provider := range attempts {
		cfg := ResolveUpstreamForProvider(provider, completionID)
		if cfg.BaseURL == "" || cfg.APIKey == "" {
			continue
		}
		triedAny = true
		url := cfg.BaseURL + "/chat/completions"

		resp, err := s.upstream.OpenStream(ctx, http.MethodPost, url, cfg.APIKey, reqBody)
		if err != nil {
			lastErr = err
			lastStatus = http.StatusBadGateway
			lastCT = "application/json"
			lastBody = nil
			continue
		}

		ct := contentTypeOrJSON(resp.Header.Get("Content-Type"))
		if resp.StatusCode != http.StatusOK {
			b, _ := io.ReadAll(resp.Body)
			_ = resp.Body.Close()

			lastStatus = resp.StatusCode
			lastCT = ct
			lastBody = b
			lastErr = nil

			if ShouldFallbackForStatus(resp.StatusCode) {
				continue
			}
			return nil, resp.StatusCode, ct, b, true
		}

		return &StreamResult{Resp: resp, CT: ct}, 0, "", nil, true
	}

	if !triedAny {
		return nil, 0, "", nil, false
	}

	if lastBody == nil {
		msg := "Upstream request failed"
		if lastErr != nil {
			msg += ": " + lastErr.Error()
		}
		b, _ := json.Marshal(map[string]any{
			"error": map[string]any{
				"message": msg,
				"type":    "upstream_error",
			},
		})
		lastBody = b
		lastCT = "application/json"
		if lastStatus == 0 {
			lastStatus = http.StatusBadGateway
		}
	}

	return nil, lastStatus, contentTypeOrJSON(lastCT), lastBody, true
}
