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
