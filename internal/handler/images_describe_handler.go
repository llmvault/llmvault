package handler

import (
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/storage"
	"github.com/usehivy/hivy/internal/system"
)

type ImageDescribeHandler struct {
	db                  *gorm.DB
	kms                 *crypto.KeyWrapper
	runtimeSecrets      *crypto.SymmetricKey
	registry            *registry.Registry
	gateway             system.Gateway
	assetReader         storage.Reader
	assetPreviewBaseURL string
}

func NewImageDescribeHandler(db *gorm.DB, kms *crypto.KeyWrapper, reg *registry.Registry, gateway system.Gateway, assetPreviewBaseURL string) *ImageDescribeHandler {
	if reg == nil {
		reg = registry.Global()
	}
	return &ImageDescribeHandler{
		db:                  db,
		kms:                 kms,
		registry:            reg,
		gateway:             gateway,
		assetPreviewBaseURL: strings.TrimRight(strings.TrimSpace(assetPreviewBaseURL), "/"),
	}
}

func (h *ImageDescribeHandler) WithAssetReader(reader storage.Reader) *ImageDescribeHandler {
	h.assetReader = reader
	return h
}

func (h *ImageDescribeHandler) WithRuntimeSecretKey(key *crypto.SymmetricKey) *ImageDescribeHandler {
	h.runtimeSecrets = key
	return h
}

type imageDescribeRequest struct {
	DriveAssetID string `json:"drive_asset_id"`
	DetailLevel  string `json:"detail_level,omitempty"`
}

type imageDescribeResponse struct {
	DriveAssetID        string         `json:"drive_asset_id"`
	AssetURL            string         `json:"asset_url"`
	Filename            string         `json:"filename"`
	ContentType         string         `json:"content_type"`
	Category            string         `json:"category"`
	Confidence          float64        `json:"confidence"`
	Analysis            map[string]any `json:"analysis"`
	RenderedDescription string         `json:"rendered_description"`
}

type imageDescribeError struct {
	Error     string `json:"error"`
	ErrorCode string `json:"error_code,omitempty"`
}
