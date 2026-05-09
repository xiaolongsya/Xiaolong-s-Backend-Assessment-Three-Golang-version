package service

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"openai-backend/internal/model"
	"openai-backend/internal/repo"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupModelServiceTestDB(t *testing.T) {
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

func TestModelService_ListDTOsAndUpdate(t *testing.T) {
	setupModelServiceTestDB(t)

	repoLayer := repo.NewAIModelRepo()
	if err := model.DB.Create(&model.AIModel{ModelID: "mini", OwnedBy: "minimax", Enabled: true, Created: time.Unix(100, 0)}).Error; err != nil {
		t.Fatalf("insert mini: %v", err)
	}
	if err := model.DB.Create(&model.AIModel{ModelID: "vol", OwnedBy: "volcano", Enabled: true, Created: time.Unix(200, 0)}).Error; err != nil {
		t.Fatalf("insert vol: %v", err)
	}
	if err := model.DB.Model(&model.AIModel{}).Where("model_id = ?", "vol").Update("enabled", false).Error; err != nil {
		t.Fatalf("disable vol: %v", err)
	}

	svc := NewModelService(repoLayer)

	enabledDTO, err := svc.ListEnabledModelsDTO()
	if err != nil {
		t.Fatalf("ListEnabledModelsDTO: %v", err)
	}
	if enabledDTO.Object != "list" || len(enabledDTO.Data) != 1 {
		t.Fatalf("enabledDTO = %#v", enabledDTO)
	}
	if enabledDTO.Data[0].ID != "mini" || enabledDTO.Data[0].Enabled != nil {
		t.Fatalf("enabledDTO.Data[0] = %#v", enabledDTO.Data[0])
	}

	allDTO, err := svc.ListAllModelsDTO()
	if err != nil {
		t.Fatalf("ListAllModelsDTO: %v", err)
	}
	if len(allDTO.Data) != 2 {
		t.Fatalf("allDTO len = %d", len(allDTO.Data))
	}
	if allDTO.Data[0].Enabled == nil || *allDTO.Data[0].Enabled != true {
		t.Fatalf("allDTO.Data[0].Enabled = %#v", allDTO.Data[0].Enabled)
	}

	updatedDTO, err := svc.UpdateEnabledDTO("vol", true)
	if err != nil {
		t.Fatalf("UpdateEnabledDTO: %v", err)
	}
	if updatedDTO.ID != "vol" || updatedDTO.Enabled == nil || *updatedDTO.Enabled != true {
		t.Fatalf("updatedDTO = %#v", updatedDTO)
	}

	updatedRow, err := svc.UpdateEnabled("vol", false)
	if err != nil {
		t.Fatalf("UpdateEnabled: %v", err)
	}
	if updatedRow.Enabled {
		t.Fatalf("expected updatedRow.Enabled=false")
	}
}
