package handler

import (
	"net/http"
	"openai-backend/internal/model"
	"time"

	"github.com/gin-gonic/gin"
)

type ModelObject struct {
	ID      string    `json:"id"`
	Object  string    `json:"object"`
	Created time.Time `json:"created"`
	OwnedBy string    `json:"owned_by"`
	Enabled bool      `json:"enabled"`
}

type ModelListResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

type UpdateModelEnabledRequest struct {
	Enabled *bool `json:"enabled"` // 用指针：便于校验必须传
}

// ListModels 返回当前可用模型列表（来源 ai_models，且仅包含 enabled=1），用于 SDK 侧 models.list 与 chat 的白名单来源。
func ListModels(c *gin.Context) {
	var rows []model.AIModel
	if err := model.DB.
		Where("enabled = ?", true).
		Order("id asc").
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to list models",
				"type":    "internal_server_error",
			},
		})
		return
	}

	data := make([]ModelObject, 0, len(rows))
	for _, r := range rows {
		data = append(data, ModelObject{
			ID:      r.ModelID,
			Object:  "model",
			Created: r.Created,
			OwnedBy: r.OwnedBy,
			Enabled: r.Enabled,
		})
	}

	c.JSON(http.StatusOK, ModelListResponse{
		Object: "list",
		Data:   data,
	})
}

// ListAllModels 返回所有模型（包含 enabled=0/1），用于后台/运维管理。
func ListAllModels(c *gin.Context) {
	var rows []model.AIModel
	if err := model.DB.
		Order("id asc").
		Find(&rows).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to list models",
				"type":    "internal_server_error",
			},
		})
		return
	}

	data := make([]ModelObject, 0, len(rows))
	for _, r := range rows {
		data = append(data, ModelObject{
			ID:      r.ModelID,
			Object:  "model",
			Created: r.Created,
			OwnedBy: r.OwnedBy,
			Enabled: r.Enabled,
		})
	}

	c.JSON(http.StatusOK, ModelListResponse{
		Object: "list",
		Data:   data,
	})
}

func UpdateModelEnabled(c *gin.Context) {
	modelID := c.Param("model_id")
	var req UpdateModelEnabledRequest

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body",
				"type":    "invalid_request_error",
			},
		})
		return
	}
	if req.Enabled == nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": gin.H{
				"message": "Invalid request body: enabled is required",
				"type":    "invalid_request_error",
			},
		})
		return
	}
	var row model.AIModel
	if err := model.DB.Where("model_id = ?", modelID).First(&row).Error; err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": gin.H{
				"message": "Model not found",
				"type":    "not_found",
			},
		})
		return
	}
	if err := model.DB.Model(&row).Update("enabled", *req.Enabled).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to update model",
				"type":    "internal_server_error",
			},
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":       row.ModelID,
		"object":   "model",
		"created":  row.Created,
		"owned_by": row.OwnedBy,
		"enabled":  *req.Enabled,
	})
}
