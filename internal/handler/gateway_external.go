package handler

import (
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/gateway"
)

type GatewayExternalHandler struct {
	db        *gorm.DB
	service   *gateway.Service
	encKey    *crypto.SymmetricKey
	enqueuer  enqueue.TaskEnqueuer
	publicURL string
	maxBytes  int64
}

func NewGatewayExternalHandler(db *gorm.DB, service *gateway.Service, encKey *crypto.SymmetricKey, enqueuer enqueue.TaskEnqueuer, publicURL string) *GatewayExternalHandler {
	return &GatewayExternalHandler{
		db:        db,
		service:   service,
		encKey:    encKey,
		enqueuer:  enqueuer,
		publicURL: strings.TrimRight(publicURL, "/"),
		maxBytes:  512 * 1024,
	}
}

type createGatewayRouteRequest struct {
	Name        string `json:"name"`
	Provider    string `json:"provider"`
	CallbackURL string `json:"callback_url"`
	Enabled     *bool  `json:"enabled,omitempty"`
}

type updateGatewayRouteRequest struct {
	Name        *string `json:"name,omitempty"`
	Provider    *string `json:"provider,omitempty"`
	CallbackURL *string `json:"callback_url,omitempty"`
	Enabled     *bool   `json:"enabled,omitempty"`
}

type gatewayRouteResponse struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	Provider     string     `json:"provider"`
	InboundURL   string     `json:"inbound_url"`
	CallbackURL  string     `json:"callback_url"`
	Secret       string     `json:"secret,omitempty"`
	SecretPrefix string     `json:"secret_prefix"`
	Enabled      bool       `json:"enabled"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	RevokedAt    *time.Time `json:"revoked_at,omitempty"`
}

func writeMissingOrg(w http.ResponseWriter) {
	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
}
