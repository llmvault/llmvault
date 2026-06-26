package handler

import (
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/storage"
)

type UploadsHandler struct {
	db                  *gorm.DB
	presigner           storage.Presigner
	streamer            storage.Streamer
	encKey              *crypto.SymmetricKey
	imageKMS            *crypto.KeyWrapper
	imageRegistry       *registry.Registry
	imageHTTPClient     *http.Client
	usageEnqueuer       enqueue.TaskEnqueuer
	agentRuntimeImage   string
	assetPreviewBaseURL string
}

const assetURLStorageColumn = "public_" + "url"

func NewUploadsHandler(db *gorm.DB, presigner storage.Presigner) *UploadsHandler {
	return &UploadsHandler{db: db, presigner: presigner}
}

func (h *UploadsHandler) WithAssetPreviewBaseURL(baseURL string) *UploadsHandler {
	h.assetPreviewBaseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	return h
}

func (h *UploadsHandler) AssetReader() storage.Reader {
	if reader, ok := h.presigner.(storage.Reader); ok {
		return reader
	}
	return nil
}

// WithStreamer enables the agent-facing streaming upload endpoint. The
// streamer is normally the same S3Presigner that satisfies both interfaces;
// the encryption key is needed to verify the runtime bearer token.
func (h *UploadsHandler) WithStreamer(s storage.Streamer, encKey *crypto.SymmetricKey) *UploadsHandler {
	h.streamer = s
	h.encKey = encKey
	return h
}

func (h *UploadsHandler) WithRuntimeImages(agentImage string) *UploadsHandler {
	h.agentRuntimeImage = agentImage
	return h
}

func (h *UploadsHandler) WithUsageEnqueuer(enq enqueue.TaskEnqueuer) *UploadsHandler {
	h.usageEnqueuer = enq
	return h
}

type signUploadRequest struct {
	AssetType   string  `json:"asset_type"`
	ContentType string  `json:"content_type"`
	SizeBytes   int64   `json:"size_bytes"`
	Filename    string  `json:"filename,omitempty"`
	OrgID       *string `json:"org_id,omitempty"`
}

type signUploadResponse struct {
	UploadURL       string            `json:"upload_url"`
	UploadMethod    string            `json:"upload_method"`
	RequiredHeaders map[string]string `json:"required_headers"`
	Key             string            `json:"key"`
	PublicURL       string            `json:"asset_url"`
	ExpiresAt       string            `json:"expires_at"`
	MaxSizeBytes    int64             `json:"max_size_bytes"`
}
