package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type ModelObject struct {
	ID      string `json:"id"`
	Object  string `json:"object"`
	Created int64  `json:"created"`
	OwnedBy string `json:"owned_by"`
}

type ModelListResponse struct {
	Object string        `json:"object"`
	Data   []ModelObject `json:"data"`
}

func ListModels(c *gin.Context) {
	now := time.Now().Unix()

	c.JSON(http.StatusOK, ModelListResponse{
		Object: "list",
		Data: []ModelObject{
			{
				ID:      "gpt-4o-mini",
				Object:  "model",
				Created: now,
				OwnedBy: "organization",
			},
		},
	})
}
