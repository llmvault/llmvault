package handler

import "github.com/google/uuid"

type streamAssetResponse struct {
	ID          uuid.UUID `json:"id"`
	PublicURL   string    `json:"asset_url"`
	Key         string    `json:"key"`
	Path        string    `json:"path"`
	Filename    string    `json:"filename"`
	ContentType string    `json:"content_type"`
	Bytes       int64     `json:"bytes"`
}

type moveAssetRequest struct {
	Asset   string `json:"asset"`
	NewPath string `json:"new_path"`
}

type moveAssetResponse struct {
	ID        uuid.UUID `json:"id"`
	PublicURL string    `json:"asset_url"`
	Key       string    `json:"key"`
	Path      string    `json:"path"`
	Filename  string    `json:"filename"`
	UpdatedAt string    `json:"updated_at"`
}
