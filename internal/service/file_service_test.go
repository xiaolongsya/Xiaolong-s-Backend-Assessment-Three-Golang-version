package service

import (
	"bytes"
	"fmt"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openai-backend/internal/model"
	"openai-backend/internal/repo"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFileServiceTestDB(t *testing.T) {
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

func newMultipartFileHeader(t *testing.T, filename string, content []byte) *multipart.FileHeader {
	t.Helper()

	path := filepath.Join(t.TempDir(), filename)
	if err := os.WriteFile(path, content, 0o644); err != nil {
		t.Fatalf("write temp file: %v", err)
	}

	file, err := os.Open(path)
	if err != nil {
		t.Fatalf("open temp file: %v", err)
	}
	defer func() { _ = file.Close() }()

	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write form file: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}

	reader := multipart.NewReader(bytes.NewReader(buf.Bytes()), writer.Boundary())
	form, err := reader.ReadForm(int64(len(content) + 1024))
	if err != nil {
		t.Fatalf("read form: %v", err)
	}
	files := form.File["file"]
	if len(files) == 0 {
		t.Fatalf("expected file header")
	}
	return files[0]
}

func TestFileService_SaveUpload_List_Get_Delete(t *testing.T) {
	setupFileServiceTestDB(t)

	storageDir := t.TempDir()
	oldStorage, hadStorage := os.LookupEnv("FILE_STORAGE_DIR")
	_ = os.Setenv("FILE_STORAGE_DIR", storageDir)
	t.Cleanup(func() {
		if hadStorage {
			_ = os.Setenv("FILE_STORAGE_DIR", oldStorage)
		} else {
			_ = os.Unsetenv("FILE_STORAGE_DIR")
		}
	})

	svc := NewFileService(repo.NewFileRepo())
	fh := newMultipartFileHeader(t, "hello.txt", []byte("hello world"))

	obj, err := svc.SaveUpload(fh, "assistants")
	if err != nil {
		t.Fatalf("SaveUpload: %v", err)
	}
	if obj.ID == "" || obj.Object != "file" || obj.Bytes != int64(len("hello world")) {
		t.Fatalf("unexpected file dto: %#v", obj)
	}

	if _, err := os.Stat(filepath.Join(storageDir, obj.ID+".txt")); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}

	list, err := svc.ListDTO()
	if err != nil {
		t.Fatalf("ListDTO: %v", err)
	}
	if list.Object != "list" || len(list.Data) != 1 {
		t.Fatalf("unexpected list dto: %#v", list)
	}

	got, err := svc.GetDTO(obj.ID)
	if err != nil {
		t.Fatalf("GetDTO: %v", err)
	}
	if got.ID != obj.ID || got.Filename != "hello.txt" {
		t.Fatalf("unexpected got dto: %#v", got)
	}

	deleted, err := svc.Delete(obj.ID)
	if err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted.Deleted || deleted.ID != obj.ID {
		t.Fatalf("unexpected delete dto: %#v", deleted)
	}
	if _, err := os.Stat(filepath.Join(storageDir, obj.ID+".txt")); !os.IsNotExist(err) {
		t.Fatalf("expected file removed from disk, stat err=%v", err)
	}
	if _, err := svc.GetDTO(obj.ID); err == nil {
		t.Fatalf("expected not found after delete")
	}
}

func TestFileService_SaveUpload_RollsBackDiskOnDBFailure(t *testing.T) {
	setupFileServiceTestDB(t)

	storageDir := t.TempDir()
	oldStorage, hadStorage := os.LookupEnv("FILE_STORAGE_DIR")
	_ = os.Setenv("FILE_STORAGE_DIR", storageDir)
	t.Cleanup(func() {
		if hadStorage {
			_ = os.Setenv("FILE_STORAGE_DIR", oldStorage)
		} else {
			_ = os.Unsetenv("FILE_STORAGE_DIR")
		}
	})

	if err := model.DB.Migrator().DropTable(&model.File{}); err != nil {
		t.Fatalf("drop files table: %v", err)
	}

	svc := NewFileService(repo.NewFileRepo())
	fh := newMultipartFileHeader(t, "broken.txt", []byte("rollback me"))
	obj, err := svc.SaveUpload(fh, "assistants")
	if err == nil {
		t.Fatalf("expected DB failure, got obj=%#v", obj)
	}
	if entries, _ := os.ReadDir(storageDir); len(entries) != 0 {
		t.Fatalf("expected disk rollback, found %d entries", len(entries))
	}
}
