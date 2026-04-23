package handler

import (
	"errors"
	"net/http"
	"openai-backend/internal/repo"
	"openai-backend/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

var fileSvc = service.NewFileService(repo.NewFileRepo())

// CreateFile handles POST /v1/files (multipart/form-data: file, purpose)
func CreateFile(c *gin.Context) {
	fh, err := c.FormFile("file")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "file is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}

	purpose := c.PostForm("purpose") // 允许为空：你选择“放开字符串”
	obj, err := fileSvc.SaveUpload(fh, purpose)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to upload file",
				"type":    "internal_server_error",
			},
		})
		return
	}

	c.JSON(http.StatusOK, obj)
}

// ListFiles handles GET /v1/files
func ListFiles(c *gin.Context) {
	resp, err := fileSvc.ListDTO()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to list files",
				"type":    "internal_server_error",
			},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// GetFile handles GET /v1/files/{file_id}
func GetFile(c *gin.Context) {
	fileID := c.Param("file_id")
	obj, err := fileSvc.GetDTO(fileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "File not found",
					"type":    "not_found",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to get file",
				"type":    "internal_server_error",
			},
		})
		return
	}
	c.JSON(http.StatusOK, obj)
}

// DeleteFile handles DELETE /v1/files/{file_id}
func DeleteFile(c *gin.Context) {
	fileID := c.Param("file_id")
	resp, err := fileSvc.Delete(fileID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "File not found",
					"type":    "not_found",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to delete file",
				"type":    "internal_server_error",
			},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}
