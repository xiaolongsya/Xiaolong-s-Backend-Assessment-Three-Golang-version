package service

import (
	"errors"
	"openai-backend/internal/dto"
	"openai-backend/internal/model"
	"openai-backend/internal/repo"

	"gorm.io/gorm"
)

type ModelService struct {
	repo *repo.AIModelRepo
}

func NewModelService(repo *repo.AIModelRepo) *ModelService {
	return &ModelService{repo: repo}
}

func toModelObject(m model.AIModel, includeEnabled bool) dto.ModelObject {
	var enabled *bool
	if includeEnabled {
		e := m.Enabled
		enabled = &e
	}
	return dto.ModelObject{
		ID:      m.ModelID,
		Object:  "model",
		Created: m.Created.Unix(),
		OwnedBy: m.OwnedBy,
		Enabled: enabled,
	}
}

func (s *ModelService) ListEnabledModels() ([]model.AIModel, error) {
	return s.repo.ListEnabled()
}

func (s *ModelService) ListAllModels() ([]model.AIModel, error) {
	return s.repo.ListAll()
}

func (s *ModelService) ListEnabledModelsDTO() (dto.ModelListResponse, error) {
	rows, err := s.repo.ListEnabled()
	if err != nil {
		return dto.ModelListResponse{}, err
	}
	data := make([]dto.ModelObject, 0, len(rows))
	for _, r := range rows {
		data = append(data, toModelObject(r, false))
	}
	return dto.ModelListResponse{Object: "list", Data: data}, nil
}

func (s *ModelService) ListAllModelsDTO() (dto.ModelListResponse, error) {
	rows, err := s.repo.ListAll()
	if err != nil {
		return dto.ModelListResponse{}, err
	}
	data := make([]dto.ModelObject, 0, len(rows))
	for _, r := range rows {
		data = append(data, toModelObject(r, true))
	}
	return dto.ModelListResponse{Object: "list", Data: data}, nil
}

func (s *ModelService) UpdateEnabledDTO(modelID string, enabled bool) (dto.ModelObject, error) {
	m, err := s.UpdateEnabled(modelID, enabled)
	if err != nil {
		return dto.ModelObject{}, err
	}
	return toModelObject(m, true), nil
}

func (s *ModelService) UpdateEnabled(modelID string, enabled bool) (model.AIModel, error) {
	m, err := s.repo.UpdateEnabled(modelID, enabled)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.AIModel{}, gorm.ErrRecordNotFound
		}
		return model.AIModel{}, err
	}
	return m, nil
}
