package handler

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"openai-backend/internal/model"
	"openai-backend/internal/task"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupCompletionHandlerDB(t *testing.T) {
	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.ReplaceAll(t.Name(), "/", "_"))
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	if err := db.AutoMigrate(&model.Completion{}, &model.AIModel{}, &model.File{}); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	model.DB = db
}

func TestGetDeleteCompletionHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCompletionHandlerDB(t)

	completion := &model.Completion{CompletionID: "chatcmpl-1", Request: `{"model":"x"}`, Response: `{"id":"chatcmpl-1","object":"chat.completion"}`, Status: "completed"}
	if err := model.DB.Create(completion).Error; err != nil {
		t.Fatalf("insert completion: %v", err)
	}

	r := gin.New()
	r.GET("/v1/chat/completions/:id", GetCompletion)
	r.DELETE("/v1/chat/completions/:id", DeleteCompletion)

	getReq := httptest.NewRequest(http.MethodGet, "/v1/chat/completions/chatcmpl-1", nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GetCompletion status = %d, body=%s", getW.Code, getW.Body.String())
	}
	if !strings.Contains(getW.Body.String(), "chatcmpl-1") {
		t.Fatalf("unexpected get body: %s", getW.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/chat/completions/chatcmpl-1", nil)
	deleteW := httptest.NewRecorder()
	r.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("DeleteCompletion status = %d, body=%s", deleteW.Code, deleteW.Body.String())
	}

	notFoundReq := httptest.NewRequest(http.MethodGet, "/v1/chat/completions/missing", nil)
	notFoundW := httptest.NewRecorder()
	r.ServeHTTP(notFoundW, notFoundReq)
	if notFoundW.Code != http.StatusNotFound {
		t.Fatalf("missing GetCompletion status = %d, body=%s", notFoundW.Code, notFoundW.Body.String())
	}

	notFoundDeleteReq := httptest.NewRequest(http.MethodDelete, "/v1/chat/completions/missing", nil)
	notFoundDeleteW := httptest.NewRecorder()
	r.ServeHTTP(notFoundDeleteW, notFoundDeleteReq)
	if notFoundDeleteW.Code != http.StatusNotFound {
		t.Fatalf("missing DeleteCompletion status = %d, body=%s", notFoundDeleteW.Code, notFoundDeleteW.Body.String())
	}
}

func TestCancelCompletionHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupCompletionHandlerDB(t)

	r := gin.New()
	r.POST("/v1/chat/completions/:id/cancel", CancelCompletion)

	called := false
	task.Register("chatcmpl-task", func() { called = true })

	successReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions/chatcmpl-task/cancel", nil)
	successW := httptest.NewRecorder()
	r.ServeHTTP(successW, successReq)
	if successW.Code != http.StatusOK {
		t.Fatalf("CancelCompletion status = %d, body=%s", successW.Code, successW.Body.String())
	}
	if !called {
		t.Fatalf("expected cancel func to be called")
	}

	notFoundReq := httptest.NewRequest(http.MethodPost, "/v1/chat/completions/missing/cancel", nil)
	notFoundW := httptest.NewRecorder()
	r.ServeHTTP(notFoundW, notFoundReq)
	if notFoundW.Code != http.StatusNotFound {
		t.Fatalf("missing CancelCompletion status = %d, body=%s", notFoundW.Code, notFoundW.Body.String())
	}
}
