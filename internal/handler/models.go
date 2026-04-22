package handler

import (
	"net/http"
	"openai-backend/internal/repo"
	"openai-backend/internal/service"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type UpdateModelEnabledRequest struct {
	Enabled *bool `json:"enabled"` // 用指针：便于校验必须传
}

var modelSvc = service.NewModelService(repo.NewAIModelRepo())

// ListModels 返回当前可用模型列表（来源 ai_models，且仅包含 enabled=1），用于 SDK 侧 models.list 与 chat 的白名单来源。
func ListModels(c *gin.Context) {
	resp, err := modelSvc.ListEnabledModelsDTO()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to list models",
				"type":    "internal_server_error",
			},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// ListAllModels 返回所有模型（包含 enabled=0/1），用于后台/运维管理。
func ListAllModels(c *gin.Context) {
	resp, err := modelSvc.ListAllModelsDTO()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to list models",
				"type":    "internal_server_error",
			},
		})
		return
	}
	c.JSON(http.StatusOK, resp)
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
	row, err := modelSvc.UpdateEnabledDTO(modelID, *req.Enabled)
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": gin.H{
					"message": "Model not found",
					"type":    "not_found",
				},
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": gin.H{
				"message": "Failed to update model",
				"type":    "internal_server_error",
			},
		})
		return
	}
	c.JSON(http.StatusOK, row)
}
