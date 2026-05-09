package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"openai-backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupHandlerModelsDB(t *testing.T) {
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

func TestListModelsAndListAllModels(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupHandlerModelsDB(t)

	rows := []model.AIModel{
		{ModelID: "alpha", OwnedBy: "minimax", Enabled: true, Created: time.Unix(100, 0)},
		{ModelID: "beta", OwnedBy: "volcano", Enabled: false, Created: time.Unix(200, 0)},
	}
	for _, row := range rows {
		if err := model.DB.Create(&row).Error; err != nil {
			t.Fatalf("insert model: %v", err)
		}
	}

	r := gin.New()
	r.GET("/v1/models", ListModels)
	r.GET("/v1/admin/models", ListAllModels)

	listReq := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	listW := httptest.NewRecorder()
	r.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("ListModels status = %d, body=%s", listW.Code, listW.Body.String())
	}
	var listResp map[string]any
	if err := json.Unmarshal(listW.Body.Bytes(), &listResp); err != nil {
		t.Fatalf("unmarshal list: %v", err)
	}
	if listResp["object"] != "list" {
		t.Fatalf("list object = %#v", listResp["object"])
	}

	adminReq := httptest.NewRequest(http.MethodGet, "/v1/admin/models", nil)
	adminW := httptest.NewRecorder()
	r.ServeHTTP(adminW, adminReq)
	if adminW.Code != http.StatusOK {
		t.Fatalf("ListAllModels status = %d, body=%s", adminW.Code, adminW.Body.String())
	}
	var adminResp map[string]any
	if err := json.Unmarshal(adminW.Body.Bytes(), &adminResp); err != nil {
		t.Fatalf("unmarshal admin: %v", err)
	}
	data := adminResp["data"].([]any)
	if len(data) != 2 {
		t.Fatalf("expected 2 models, got %d", len(data))
	}
}

func TestUpdateModelEnabled(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupHandlerModelsDB(t)

	if err := model.DB.Create(&model.AIModel{ModelID: "alpha", OwnedBy: "minimax", Enabled: true, Created: time.Unix(100, 0)}).Error; err != nil {
		t.Fatalf("insert model: %v", err)
	}

	r := gin.New()
	r.PATCH("/v1/admin/models/:model_id", UpdateModelEnabled)

	// missing enabled -> 400
	missingReq := httptest.NewRequest(http.MethodPatch, "/v1/admin/models/alpha", bytes.NewReader([]byte(`{}`)))
	missingReq.Header.Set("Content-Type", "application/json")
	missingW := httptest.NewRecorder()
	r.ServeHTTP(missingW, missingReq)
	if missingW.Code != http.StatusBadRequest {
		t.Fatalf("missing enabled status = %d, body=%s", missingW.Code, missingW.Body.String())
	}

	// success -> 200
	req := httptest.NewRequest(http.MethodPatch, "/v1/admin/models/alpha", bytes.NewReader([]byte(`{"enabled":false}`)))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("update status = %d, body=%s", w.Code, w.Body.String())
	}

	var updated map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &updated); err != nil {
		t.Fatalf("unmarshal updated: %v", err)
	}
	if updated["id"] != "alpha" {
		t.Fatalf("updated id = %#v", updated["id"])
	}

	// not found -> 404
	notFoundReq := httptest.NewRequest(http.MethodPatch, "/v1/admin/models/not-exist", bytes.NewReader([]byte(`{"enabled":true}`)))
	notFoundReq.Header.Set("Content-Type", "application/json")
	notFoundW := httptest.NewRecorder()
	r.ServeHTTP(notFoundW, notFoundReq)
	if notFoundW.Code != http.StatusNotFound {
		t.Fatalf("not found status = %d, body=%s", notFoundW.Code, notFoundW.Body.String())
	}
}
