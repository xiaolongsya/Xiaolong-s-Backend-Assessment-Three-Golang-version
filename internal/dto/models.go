package dto

import "time"

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
