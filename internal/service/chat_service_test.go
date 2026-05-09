package service

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"openai-backend/internal/model"
	"openai-backend/internal/repo"
	"openai-backend/internal/upstream"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupServiceTestDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Completion{}, &model.AIModel{}, &model.File{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.DB = db
}

func setEnv(t *testing.T, key, value string) {
	old, had := os.LookupEnv(key)
	if value == "" {
		_ = os.Unsetenv(key)
	} else {
		_ = os.Setenv(key, value)
	}
	t.Cleanup(func() {
		if had {
			_ = os.Setenv(key, old)
		} else {
			_ = os.Unsetenv(key)
		}
	})
}

func TestProxyNonStream_FallbackToSecondProvider(t *testing.T) {
	setupServiceTestDB(t)

	setEnv(t, "UPSTREAM_FALLBACKS", "volcano=minimax")

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("primary path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer primary-key" {
			t.Fatalf("primary auth = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"error":{"message":"primary down"}}`))
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		if r.URL.Path != "/chat/completions" {
			t.Fatalf("secondary path = %s", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "Bearer secondary-key" {
			t.Fatalf("secondary auth = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"upstream-id","object":"chat.completion","choices":[{"index":0,"message":{"role":"assistant","content":"ok"},"finish_reason":"stop"}]}`))
	}))
	defer secondary.Close()

	setEnv(t, "UPSTREAM_VOLCANO_BASE_URL", primary.URL)
	setEnv(t, "UPSTREAM_VOLCANO_API_KEY", "primary-key")
	setEnv(t, "UPSTREAM_MINIMAX_BASE_URL", secondary.URL)
	setEnv(t, "UPSTREAM_MINIMAX_API_KEY", "secondary-key")
	setEnv(t, "UPSTREAM_BASE_URL", "")
	setEnv(t, "UPSTREAM_API_KEY", "")

	chat := NewChatService(repo.NewAIModelRepo(), repo.NewCompletionRepo(), upstream.NewClient(&http.Client{Timeout: 5 * time.Second}))

	completionID := "chatcmpl-test"
	attempts := chat.BuildAttempts("volcano")
	reqBody := []byte(`{"model":"x","messages":[{"role":"user","content":"hi"}],"stream":false}`)

	status, ct, body, ok := chat.ProxyNonStream(context.Background(), attempts, completionID, reqBody)
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if status != http.StatusOK {
		t.Fatalf("status = %d, want %d", status, http.StatusOK)
	}
	if ct != "application/json" {
		t.Fatalf("content-type = %q, want application/json", ct)
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	if secondaryCalls != 1 {
		t.Fatalf("secondaryCalls = %d, want 1", secondaryCalls)
	}

	var resp map[string]any
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["id"] != completionID {
		t.Fatalf("response id = %#v, want %q", resp["id"], completionID)
	}
}

func TestProxyNonStream_NoFallbackOnClientErrorStatus(t *testing.T) {
	setupServiceTestDB(t)

	setEnv(t, "UPSTREAM_FALLBACKS", "volcano=minimax")

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"message":"bad request"}}`))
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"id":"should-not-hit"}`))
	}))
	defer secondary.Close()

	setEnv(t, "UPSTREAM_VOLCANO_BASE_URL", primary.URL)
	setEnv(t, "UPSTREAM_VOLCANO_API_KEY", "primary-key")
	setEnv(t, "UPSTREAM_MINIMAX_BASE_URL", secondary.URL)
	setEnv(t, "UPSTREAM_MINIMAX_API_KEY", "secondary-key")

	chat := NewChatService(repo.NewAIModelRepo(), repo.NewCompletionRepo(), upstream.NewClient(&http.Client{Timeout: 5 * time.Second}))

	status, ct, body, ok := chat.ProxyNonStream(context.Background(), chat.BuildAttempts("volcano"), "chatcmpl-test", []byte(`{"stream":false}`))
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if status != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", status, http.StatusBadRequest)
	}
	if ct != "application/json" {
		t.Fatalf("ct = %q, want application/json", ct)
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	if secondaryCalls != 0 {
		t.Fatalf("secondaryCalls = %d, want 0", secondaryCalls)
	}
	if len(body) == 0 {
		t.Fatalf("expected response body")
	}
}

func TestOpenUpstreamStream_FallbackToSecondProvider(t *testing.T) {
	setupServiceTestDB(t)

	setEnv(t, "UPSTREAM_FALLBACKS", "volcano=minimax")

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("data: {\"error\":\"primary down\"}\n\n"))
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		if got := r.Header.Get("Authorization"); got != "Bearer secondary-key" {
			t.Fatalf("secondary auth = %q", got)
		}
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"id\":\"upstream-id\",\"choices\":[{\"delta\":{\"content\":\"hello\"}}]}\n\n"))
		_, _ = w.Write([]byte("data: [DONE]\n\n"))
	}))
	defer secondary.Close()

	setEnv(t, "UPSTREAM_VOLCANO_BASE_URL", primary.URL)
	setEnv(t, "UPSTREAM_VOLCANO_API_KEY", "primary-key")
	setEnv(t, "UPSTREAM_MINIMAX_BASE_URL", secondary.URL)
	setEnv(t, "UPSTREAM_MINIMAX_API_KEY", "secondary-key")
	setEnv(t, "UPSTREAM_BASE_URL", "")
	setEnv(t, "UPSTREAM_API_KEY", "")

	chat := NewChatService(repo.NewAIModelRepo(), repo.NewCompletionRepo(), upstream.NewClient(&http.Client{Timeout: 5 * time.Second}))
	stream, errStatus, errCT, errBody, ok := chat.OpenUpstreamStream(context.Background(), chat.BuildAttempts("volcano"), "chatcmpl-test", []byte(`{"stream":true}`))
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if stream == nil {
		t.Fatalf("expected stream result")
	}
	defer stream.Resp.Body.Close()

	if errStatus != 0 || errCT != "" || errBody != nil {
		t.Fatalf("expected no error response, got status=%d ct=%q body=%q", errStatus, errCT, string(errBody))
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	if secondaryCalls != 1 {
		t.Fatalf("secondaryCalls = %d, want 1", secondaryCalls)
	}

	body, err := io.ReadAll(stream.Resp.Body)
	if err != nil {
		t.Fatalf("read stream body: %v", err)
	}
	if len(body) == 0 {
		t.Fatalf("expected stream body")
	}
	if string(body) == "" {
		t.Fatalf("expected non-empty stream body")
	}
}

func TestOpenUpstreamStream_NoFallbackOnClientErrorStatus(t *testing.T) {
	setupServiceTestDB(t)

	setEnv(t, "UPSTREAM_FALLBACKS", "volcano=minimax")

	primaryCalls := 0
	primary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		primaryCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte("data: {\"error\":\"bad request\"}\n\n"))
	}))
	defer primary.Close()

	secondaryCalls := 0
	secondary := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		secondaryCalls++
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("data: {\"id\":\"should-not-hit\"}\n\n"))
	}))
	defer secondary.Close()

	setEnv(t, "UPSTREAM_VOLCANO_BASE_URL", primary.URL)
	setEnv(t, "UPSTREAM_VOLCANO_API_KEY", "primary-key")
	setEnv(t, "UPSTREAM_MINIMAX_BASE_URL", secondary.URL)
	setEnv(t, "UPSTREAM_MINIMAX_API_KEY", "secondary-key")

	chat := NewChatService(repo.NewAIModelRepo(), repo.NewCompletionRepo(), upstream.NewClient(&http.Client{Timeout: 5 * time.Second}))
	stream, errStatus, errCT, errBody, ok := chat.OpenUpstreamStream(context.Background(), chat.BuildAttempts("volcano"), "chatcmpl-test", []byte(`{"stream":true}`))
	if !ok {
		t.Fatalf("expected ok=true")
	}
	if stream != nil {
		t.Fatalf("expected nil stream")
	}
	if errStatus != http.StatusBadRequest {
		t.Fatalf("errStatus = %d, want %d", errStatus, http.StatusBadRequest)
	}
	if errCT != "text/event-stream" {
		t.Fatalf("errCT = %q, want text/event-stream", errCT)
	}
	if len(errBody) == 0 {
		t.Fatalf("expected error body")
	}
	if primaryCalls != 1 {
		t.Fatalf("primaryCalls = %d, want 1", primaryCalls)
	}
	if secondaryCalls != 0 {
		t.Fatalf("secondaryCalls = %d, want 0", secondaryCalls)
	}
}
