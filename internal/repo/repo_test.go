package repo

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"openai-backend/internal/model"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupRepoTestDB(t *testing.T) {
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

func TestAIModelRepo_CRUDAndListing(t *testing.T) {
	setupRepoTestDB(t)
	repo := NewAIModelRepo()

	models := []model.AIModel{
		{ModelID: "b-model", OwnedBy: "volcano", Enabled: true, Created: time.Now()},
		{ModelID: "a-model", OwnedBy: "minimax", Enabled: true, Created: time.Now()},
	}
	for _, m := range models {
		if err := model.DB.Create(&m).Error; err != nil {
			t.Fatalf("insert model: %v", err)
		}
	}
	if err := model.DB.Model(&model.AIModel{}).Where("model_id = ?", "b-model").Update("enabled", false).Error; err != nil {
		t.Fatalf("disable b-model: %v", err)
	}

	enabled, err := repo.GetEnabledByModelID("a-model")
	if err != nil {
		t.Fatalf("GetEnabledByModelID: %v", err)
	}
	if enabled.ModelID != "a-model" {
		t.Fatalf("enabled.ModelID = %q", enabled.ModelID)
	}

	if _, err := repo.GetEnabledByModelID("b-model"); err == nil {
		t.Fatalf("expected not found for disabled model")
	}

	listEnabled, err := repo.ListEnabled()
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(listEnabled) != 1 || listEnabled[0].ModelID != "a-model" {
		t.Fatalf("ListEnabled = %#v", listEnabled)
	}

	listAll, err := repo.ListAll()
	if err != nil {
		t.Fatalf("ListAll: %v", err)
	}
	if len(listAll) != 2 {
		t.Fatalf("ListAll len = %d", len(listAll))
	}

	updated, err := repo.UpdateEnabled("b-model", true)
	if err != nil {
		t.Fatalf("UpdateEnabled: %v", err)
	}
	if !updated.Enabled {
		t.Fatalf("expected updated model enabled=true")
	}
}

func TestCompletionRepo_CRUD(t *testing.T) {
	setupRepoTestDB(t)
	repo := NewCompletionRepo()

	req := []byte(`{"model":"x"}`)
	if err := repo.CreateRunning("chatcmpl-1", req); err != nil {
		t.Fatalf("CreateRunning: %v", err)
	}

	row, err := repo.GetByCompletionID("chatcmpl-1")
	if err != nil {
		t.Fatalf("GetByCompletionID: %v", err)
	}
	if row.Status != "running" {
		t.Fatalf("row.Status = %q", row.Status)
	}
	if row.Request != string(req) {
		t.Fatalf("row.Request = %q", row.Request)
	}

	if err := repo.UpdateFields("chatcmpl-1", map[string]any{"status": "completed", "response": `{"ok":true}`}); err != nil {
		t.Fatalf("UpdateFields: %v", err)
	}
	row, err = repo.GetByCompletionID("chatcmpl-1")
	if err != nil {
		t.Fatalf("GetByCompletionID after update: %v", err)
	}
	if row.Status != "completed" {
		t.Fatalf("updated row.Status = %q", row.Status)
	}
	if row.Response != `{"ok":true}` {
		t.Fatalf("updated row.Response = %q", row.Response)
	}

	if err := repo.DeleteByCompletionID("chatcmpl-1"); err != nil {
		t.Fatalf("DeleteByCompletionID: %v", err)
	}
	if _, err := repo.GetByCompletionID("chatcmpl-1"); err == nil {
		t.Fatalf("expected completion to be deleted")
	}
}
