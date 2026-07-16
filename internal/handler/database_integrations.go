package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/connectionname"
	"github.com/usehivy/hivy/internal/crypto"
	dbi "github.com/usehivy/hivy/internal/databaseintegration"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type DatabaseIntegrationHandler struct {
	db  *gorm.DB
	kms *crypto.KeyWrapper
}

func NewDatabaseIntegrationHandler(db *gorm.DB, kms *crypto.KeyWrapper) *DatabaseIntegrationHandler {
	return &DatabaseIntegrationHandler{db: db, kms: kms}
}

type databaseConnectionRequest struct {
	Provider      string     `json:"provider"`
	DisplayName   string     `json:"display_name"`
	ConnectionURL string     `json:"connection_url"`
	AccessPolicy  dbi.Policy `json:"access_policy,omitempty"`
}

type databaseConnectionResponse struct {
	ID             uuid.UUID  `json:"id"`
	Provider       string     `json:"provider"`
	DisplayName    string     `json:"display_name"`
	Name           string     `json:"name"`
	Slug           string     `json:"slug"`
	NeedsName      bool       `json:"needs_name"`
	SchemaSnapshot any        `json:"schema_snapshot,omitempty"`
	AccessPolicy   model.JSON `json:"access_policy"`
	CreatedAt      string     `json:"created_at"`
	UpdatedAt      string     `json:"updated_at"`
	RevokedAt      *string    `json:"revoked_at,omitempty"`
}

// Create handles POST /v1/database-integrations.
// @Summary Create database integration
// @Description Stores encrypted database credentials for the current organization.
// @Tags database-integrations
// @Accept json
// @Produce json
// @Param body body databaseConnectionRequest true "Database integration details"
// @Success 201 {object} databaseConnectionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/database-integrations [post]
func (h *DatabaseIntegrationHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	var req databaseConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	provider, err := dbi.NormalizeProvider(req.Provider)
	if err != nil || req.ConnectionURL == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "provider and connection_url are required"})
		return
	}
	encrypted, wrapped, err := dbi.EncryptSecret(ctx, h.kms, req.ConnectionURL)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("database integration encrypt: %w", err))
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to encrypt database credentials"})
		return
	}
	conn := model.DatabaseConnection{
		ID:           uuid.New(),
		OrgID:        org.ID,
		Provider:     provider,
		DisplayName:  req.DisplayName,
		EncryptedDSN: encrypted,
		WrappedDEK:   wrapped,
		AccessPolicy: dbi.PolicyToJSON(req.AccessPolicy),
	}
	if conn.DisplayName == "" {
		conn.DisplayName = provider
	}
	for attempt := 0; attempt < 5; attempt++ {
		err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			identity, identityErr := connectionname.ForDatabase(ctx, tx, org.ID, provider)
			if identityErr != nil {
				return identityErr
			}
			conn.Name = identity.Name
			conn.Slug = identity.Slug
			conn.NeedsName = identity.NeedsName
			return tx.WithContext(ctx).Create(&conn).Error
		})
		if err == nil || !isDuplicateKeyError(err) {
			break
		}
	}
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("database integration create: %w", err))
		if isDuplicateKeyError(err) {
			writeConnectionNameError(w, connectionname.ErrNameTaken)
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create database connection"})
		return
	}
	writeJSON(w, http.StatusCreated, databaseConnectionToResponse(conn))
}

// List handles GET /v1/database-integrations.
// @Summary List database integrations
// @Description Lists active database integrations for the current organization.
// @Tags database-integrations
// @Produce json
// @Success 200 {array} databaseConnectionResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/database-integrations [get]
func (h *DatabaseIntegrationHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}
	var rows []model.DatabaseConnection
	if err := h.db.WithContext(r.Context()).
		Where("org_id = ? AND revoked_at IS NULL", org.ID).
		Order("created_at DESC").
		Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list database integrations"})
		return
	}
	resp := make([]databaseConnectionResponse, len(rows))
	for i, row := range rows {
		resp[i] = databaseConnectionToResponse(row)
	}
	writeJSON(w, http.StatusOK, resp)
}

// Revoke handles DELETE /v1/database-integrations/{id}.
// @Summary Disconnect database integration
// @Description Revokes a database integration without exposing stored credentials.
// @Tags database-integrations
// @Produce json
// @Param id path string true "Database integration ID"
// @Success 200 {object} databaseConnectionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/database-integrations/{id} [delete]
func (h *DatabaseIntegrationHandler) Revoke(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.loadOrgConnection(w, r)
	if !ok {
		return
	}
	now := time.Now()
	if err := h.db.WithContext(r.Context()).Model(&conn).Update("revoked_at", now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to disconnect database integration"})
		return
	}
	conn.RevokedAt = &now
	writeJSON(w, http.StatusOK, databaseConnectionToResponse(conn))
}

func (h *DatabaseIntegrationHandler) loadOrgConnection(w http.ResponseWriter, r *http.Request) (model.DatabaseConnection, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return model.DatabaseConnection{}, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid database integration id"})
		return model.DatabaseConnection{}, false
	}
	var conn model.DatabaseConnection
	if err := h.db.WithContext(r.Context()).Where("id = ? AND org_id = ?", id, org.ID).First(&conn).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "database integration not found"})
		return model.DatabaseConnection{}, false
	}
	return conn, true
}

func databaseConnectionToResponse(conn model.DatabaseConnection) databaseConnectionResponse {
	var snapshot any
	_ = json.Unmarshal(conn.SchemaSnapshot, &snapshot)
	resp := databaseConnectionResponse{
		ID:             conn.ID,
		Provider:       conn.Provider,
		DisplayName:    conn.DisplayName,
		Name:           conn.Name,
		Slug:           conn.Slug,
		NeedsName:      conn.NeedsName,
		SchemaSnapshot: snapshot,
		AccessPolicy:   conn.AccessPolicy,
		CreatedAt:      conn.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      conn.UpdatedAt.Format(time.RFC3339),
	}
	if conn.RevokedAt != nil {
		s := conn.RevokedAt.Format(time.RFC3339)
		resp.RevokedAt = &s
	}
	return resp
}
