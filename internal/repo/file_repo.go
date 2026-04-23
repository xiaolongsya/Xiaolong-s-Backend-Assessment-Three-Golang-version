package repo

import (
	"errors"
	"openai-backend/internal/model"

	"gorm.io/gorm"
)

type FileRepo struct{}

func NewFileRepo() *FileRepo {
	return &FileRepo{}
}

func (r *FileRepo) Create(row *model.File) error {
	return model.DB.Create(row).Error
}

func (r *FileRepo) GetByFileID(fileID string) (model.File, error) {
	var f model.File
	if err := model.DB.Where("file_id = ?", fileID).First(&f).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return model.File{}, gorm.ErrRecordNotFound
		}
		return model.File{}, err
	}
	return f, nil
}

func (r *FileRepo) ListAll() ([]model.File, error) {
	var rows []model.File
	if err := model.DB.Order("id desc").Find(&rows).Error; err != nil {
		return nil, err
	}
	return rows, nil
}

func (r *FileRepo) DeleteByFileID(fileID string) error {
	f, err := r.GetByFileID(fileID)
	if err != nil {
		return err
	}

	return model.DB.Unscoped().Delete(&f).Error
}
