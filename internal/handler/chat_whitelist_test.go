package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"openai-backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func withEnv(t *testing.T, key, value string) {
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

func setupTestDB(t *testing.T) {
	db, err := gorm.Open(sqlite.Open("file::memory:?cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Completion{}, &model.AIModel{}, &model.File{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.DB = db
}

func TestChatCompletions_ModelNotWhitelisted_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	// Ensure upstream is disabled so we stay in mock path.
	withEnv(t, "UPSTREAM_BASE_URL", "")
	withEnv(t, "UPSTREAM_API_KEY", "")

	r := gin.New()
	r.POST("/v1/chat/completions", ChatCompletions)

	payload := map[string]any{
		"model":       "NOT-EXIST",
		"messages":    []map[string]any{{"role": "user", "content": "hi"}},
		"temperature": 0.7,
		"stream":      false,
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected %d, got %d, body=%s", http.StatusBadRequest, w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	errObj, ok := resp["error"].(map[string]any)
	if !ok {
		t.Fatalf("expected error object, got %#v", resp)
	}
	if errObj["type"] != "invalid_request_error" {
		t.Fatalf("expected invalid_request_error, got %#v", errObj["type"])
	}
}

func TestChatCompletions_WhitelistedModel_MockOK(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupTestDB(t)

	withEnv(t, "UPSTREAM_BASE_URL", "")
	withEnv(t, "UPSTREAM_API_KEY", "")

	// Insert enabled model.
	if err := model.DB.Create(&model.AIModel{
		ModelID: "MiniMax-M2.7",
		OwnedBy: "minimax",
		Enabled: true,
		Created: time.Now(),
	}).Error; err != nil {
		t.Fatalf("insert model: %v", err)
	}

	r := gin.New()
	r.POST("/v1/chat/completions", ChatCompletions)

	payload := map[string]any{
		"model":       "MiniMax-M2.7",
		"messages":    []map[string]any{{"role": "user", "content": "hi"}},
		"temperature": 0.7,
		"stream":      false,
	}
	b, _ := json.Marshal(payload)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")

	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected %d, got %d, body=%s", http.StatusOK, w.Code, w.Body.String())
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	id, _ := resp["id"].(string)
	if id == "" {
		t.Fatalf("expected non-empty id")
	}
	if resp["object"] != "chat.completion" {
		t.Fatalf("expected object chat.completion, got %#v", resp["object"])
	}
}
