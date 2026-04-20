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
}

type ModelListResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
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
		})
	}

	c.JSON(http.StatusOK, ModelListResponse{
		Object: "list",
		Data:   data,
	})
}
