package repo

import "openai-backend/internal/model"

type CompletionRepo struct{}

func NewCompletionRepo() *CompletionRepo {
	return &CompletionRepo{}
}

func (r *CompletionRepo) CreateRunning(completionID string, requestJSON []byte) error {
	return model.DB.Create(&model.Completion{
		CompletionID: completionID,
		Request:      string(requestJSON),
		Response:     "",
		Status:       "running",
	}).Error
}

func (r *CompletionRepo) UpdateFields(completionID string, fields map[string]any) error {
	return model.DB.Model(&model.Completion{}).
		Where("completion_id = ?", completionID).
		Updates(fields).Error
}
