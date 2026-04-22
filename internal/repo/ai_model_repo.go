package repo

import (
	"errors"
	"openai-backend/internal/model"

	"gorm.io/gorm"
)

type AIModelRepo struct{}

func NewAIModelRepo() *AIModelRepo {
	return &AIModelRepo{}
}

func (r *AIModelRepo) GetEnabledByModelID(modelID string) (model.AIModel, error) {
	var m model.AIModel
	if err := model.DB.
		Where("model_id = ? AND enabled = ?", modelID, true).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.AIModel{}, gorm.ErrRecordNotFound
		}
		return model.AIModel{}, err
	}
	return m, nil
}

func (r *AIModelRepo) GetByModelID(modelID string) (model.AIModel, error) {
	var m model.AIModel
	if err := model.DB.
		Where("model_id = ?", modelID).
		First(&m).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.AIModel{}, gorm.ErrRecordNotFound
		}
		return model.AIModel{}, err
	}
	return m, nil
}

func (r *AIModelRepo) ListEnabled() ([]model.AIModel, error) {
	var rows []model.AIModel
	if err := model.DB.
		Where("enabled = ?", true).
		Order("id asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *AIModelRepo) ListAll() ([]model.AIModel, error) {
	var rows []model.AIModel
	if err := model.DB.
		Order("id asc").
		Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *AIModelRepo) UpdateEnabled(modelID string, enabled bool) (model.AIModel, error) {
	m, err := r.GetByModelID(modelID)
	if err != nil {
		return model.AIModel{}, err
	}
	if err := model.DB.Model(&m).Update("enabled", enabled).Error; err != nil {
		return model.AIModel{}, err
	}
	// keep returned object consistent with DB
	m.Enabled = enabled
	return m, nil
}
