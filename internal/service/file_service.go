package service

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"time"

	"openai-backend/internal/dto"
	"openai-backend/internal/model"
	"openai-backend/internal/repo"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

type FileService struct {
	files *repo.FileRepo
}

func NewFileService(files *repo.FileRepo) *FileService {
	return &FileService{files: files}
}

func fileStorageDir() string {
	dir := strings.TrimSpace(os.Getenv("FILE_STORAGE_DIR"))
	if dir == "" {
		// 默认：项目目录下 data/files
		dir = filepath.Join(".", "data", "files")
	}
	return dir
}

func ensureDir(dir string) error {
	return os.MkdirAll(dir, 0o755)
}

func toFileDTO(f model.File) dto.FileObject {
	return dto.FileObject{
		ID:        f.FileID,
		Object:    "file",
		Bytes:     f.Bytes,
		CreatedAt: f.CreatedAt.Unix(),
		Filename:  f.Filename,
		Purpose:   f.Purpose,
	}
}

// SaveUpload saves multipart upload to disk and creates DB record.
func (s *FileService) SaveUpload(fileHeader *multipart.FileHeader, purpose string) (dto.FileObject, error) {
	if fileHeader == nil {
		return dto.FileObject{}, errors.New("file is required")
	}

	src, err := fileHeader.Open()
	if err != nil {
		return dto.FileObject{}, err
	}
	defer func() { _ = src.Close() }()

	dir := fileStorageDir()
	if err := ensureDir(dir); err != nil {
		return dto.FileObject{}, err
	}

	fileID := "file-" + uuid.NewString()

	// Safer filename: only base name
	filename := filepath.Base(strings.TrimSpace(fileHeader.Filename))
	if filename == "" {
		filename = "upload.bin"
	}

	ext := filepath.Ext(filename)

	// Write to a temp file then rename (atomic-ish)
	tmpPath := filepath.Join(dir, fileID+ext+".tmp")
	finalPath := filepath.Join(dir, fileID+ext)

	dst, err := os.Create(tmpPath)
	if err != nil {
		return dto.FileObject{}, err
	}

	hasher := sha256.New()
	n, copyErr := io.Copy(io.MultiWriter(dst, hasher), src)
	closeErr := dst.Close()

	if copyErr != nil {
		_ = os.Remove(tmpPath)
		return dto.FileObject{}, copyErr
	}
	if closeErr != nil {
		_ = os.Remove(tmpPath)
		return dto.FileObject{}, closeErr
	}

	if err := os.Rename(tmpPath, finalPath); err != nil {
		_ = os.Remove(tmpPath)
		return dto.FileObject{}, err
	}

	mimeType := strings.TrimSpace(fileHeader.Header.Get("Content-Type"))
	checksum := hex.EncodeToString(hasher.Sum(nil))

	row := &model.File{
		FileID:      fileID,
		Bytes:       n,
		Filename:    filename,
		Purpose:     strings.TrimSpace(purpose),
		MimeType:    mimeType,
		StoragePath: finalPath,
	}

	if err := s.files.Create(row); err != nil {
		// rollback disk if DB failed
		_ = os.Remove(finalPath)
		return dto.FileObject{}, err
	}

	_ = checksum // 先不入库，保留以后扩展
	return toFileDTO(*row), nil
}

func (s *FileService) ListDTO() (dto.FileListResponse, error) {
	rows, err := s.files.ListAll()
	if err != nil {
		return dto.FileListResponse{}, err
	}
	data := make([]dto.FileObject, 0, len(rows))
	for _, r := range rows {
		data = append(data, toFileDTO(r))
	}
	return dto.FileListResponse{Object: "list", Data: data}, nil
}

func (s *FileService) GetDTO(fileID string) (dto.FileObject, error) {
	row, err := s.files.GetByFileID(fileID)
	if err != nil {
		return dto.FileObject{}, err
	}
	return toFileDTO(row), nil
}

func (s *FileService) Delete(fileID string) (dto.FileDeleteResponse, error) {
	row, err := s.files.GetByFileID(fileID)
	if err != nil {
		return dto.FileDeleteResponse{}, err
	}

	// Try delete disk first, then delete DB; if disk delete fails, stop.
	if strings.TrimSpace(row.StoragePath) != "" {
		if err := os.Remove(row.StoragePath); err != nil && !os.IsNotExist(err) {
			return dto.FileDeleteResponse{}, err
		}
	}

	if err := s.files.DeleteByFileID(fileID); err != nil {
		return dto.FileDeleteResponse{}, err
	}

	return dto.FileDeleteResponse{
		ID:      fileID,
		Object:  "file",
		Deleted: true,
	}, nil
}

func IsNotFound(err error) bool {
	return errors.Is(err, gorm.ErrRecordNotFound)
}

// helper to silence unused import if time not used by your gofmt;
// keep for future extension (createdAt from gorm uses time anyway).
var _ = time.Second
