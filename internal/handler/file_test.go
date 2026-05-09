package handler

import (
	"bytes"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"openai-backend/internal/model"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func setupFileHandlerTestDB(t *testing.T) {
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

func buildMultipartRequest(t *testing.T, filename string, content []byte, purpose string) *http.Request {
	t.Helper()
	buf := &bytes.Buffer{}
	writer := multipart.NewWriter(buf)
	part, err := writer.CreateFormFile("file", filename)
	if err != nil {
		t.Fatalf("create form file: %v", err)
	}
	if _, err := part.Write(content); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if purpose != "" {
		if err := writer.WriteField("purpose", purpose); err != nil {
			t.Fatalf("write purpose: %v", err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/files", bytes.NewReader(buf.Bytes()))
	req.Header.Set("Content-Type", writer.FormDataContentType())
	return req
}

func TestCreateListGetDeleteFileHandlers(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFileHandlerTestDB(t)

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

	r := gin.New()
	r.POST("/v1/files", CreateFile)
	r.GET("/v1/files", ListFiles)
	r.GET("/v1/files/:file_id", GetFile)
	r.DELETE("/v1/files/:file_id", DeleteFile)

	uploadReq := buildMultipartRequest(t, "demo.txt", []byte("hello files"), "assistants")
	uploadW := httptest.NewRecorder()
	r.ServeHTTP(uploadW, uploadReq)
	if uploadW.Code != http.StatusOK {
		t.Fatalf("CreateFile status = %d, body=%s", uploadW.Code, uploadW.Body.String())
	}

	var uploaded map[string]any
	if err := json.Unmarshal(uploadW.Body.Bytes(), &uploaded); err != nil {
		t.Fatalf("unmarshal upload: %v", err)
	}
	fileID, _ := uploaded["id"].(string)
	if fileID == "" {
		t.Fatalf("expected file id")
	}
	if _, err := os.Stat(filepath.Join(storageDir, fileID+".txt")); err != nil {
		t.Fatalf("expected file on disk: %v", err)
	}

	listReq := httptest.NewRequest(http.MethodGet, "/v1/files", nil)
	listW := httptest.NewRecorder()
	r.ServeHTTP(listW, listReq)
	if listW.Code != http.StatusOK {
		t.Fatalf("ListFiles status = %d, body=%s", listW.Code, listW.Body.String())
	}

	getReq := httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID, nil)
	getReq = httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID, nil)
	getW := httptest.NewRecorder()
	r.ServeHTTP(getW, getReq)
	if getW.Code != http.StatusOK {
		t.Fatalf("GetFile status = %d, body=%s", getW.Code, getW.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/v1/files/"+fileID, nil)
	deleteW := httptest.NewRecorder()
	r.ServeHTTP(deleteW, deleteReq)
	if deleteW.Code != http.StatusOK {
		t.Fatalf("DeleteFile status = %d, body=%s", deleteW.Code, deleteW.Body.String())
	}

	missingReq := httptest.NewRequest(http.MethodGet, "/v1/files/"+fileID, nil)
	missingW := httptest.NewRecorder()
	r.ServeHTTP(missingW, missingReq)
	if missingW.Code != http.StatusNotFound {
		t.Fatalf("missing GetFile status = %d, body=%s", missingW.Code, missingW.Body.String())
	}
}

func TestCreateFile_MissingFile_Returns400(t *testing.T) {
	gin.SetMode(gin.TestMode)
	setupFileHandlerTestDB(t)

	r := gin.New()
	r.POST("/v1/files", CreateFile)

	req := httptest.NewRequest(http.MethodPost, "/v1/files", bytes.NewReader(nil))
	req.Header.Set("Content-Type", "multipart/form-data; boundary=missing")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d, body=%s", w.Code, w.Body.String())
	}
}
