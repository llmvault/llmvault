package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// @Summary Create an connection
// @Description Stores a connection after the OAuth flow completes via Nango.
// @Tags connections
// @Accept json
// @Produce json
// @Param id path string true "Integration ID"
// @Param body body createConnectionRequest true "Connection details"
// @Success 201 {object} connectionResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/integrations/{id}/connections [post]
func (h *ConnectionHandler) Create(w http.ResponseWriter, r *http.Request) {
	user, ok := middleware.UserFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing user context"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	integID := chi.URLParam(r, "id")
	if integID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "integration id required"})
		return
	}

	integUUID, err := uuid.Parse(integID)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid integration id"})
		return
	}

	var integ model.Integration
	if err := h.db.Where("id = ? AND deleted_at IS NULL", integUUID).First(&integ).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "integration not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to find integration"})
		return
	}

	var req createConnectionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.NangoConnectionID == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "nango_connection_id is required"})
		return
	}

	// Verify the client-supplied nango_connection_id actually exists in Nango
	// under THIS org's integration before trusting it. Without this an attacker
	// could persist a connection row whose id collides with another org's
	// connection, misrouting the other org's forwarded webhooks (and their
	// payloads) into this org. GetConnection is scoped by provider_config_key
	// (the integration's unique key), so a forged or foreign id fails here.
	if h.nango != nil {
		if _, err := h.nango.GetConnection(r.Context(), req.NangoConnectionID, nangoProviderConfigKey(integ.UniqueKey)); err != nil {
			logging.FromContext(r.Context()).WarnContext(r.Context(), "connection create rejected: nango connection not verified",
				"org_id", org.ID, "integration_id", integ.ID, "provider", integ.Provider, "error", err)
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "connection could not be verified with the provider"})
			return
		}
	}

	meta := req.Meta
	if meta == nil {
		meta = model.JSON{}
	}
	delete(meta, "resources")
	delete(meta, "credentials")

	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             org.ID,
		UserID:            user.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: req.NangoConnectionID,
		Meta:              meta,
		WebhookConfigured: boolPtr(!providerRequiresWebhookConfig(integ.Provider)),
	}

	err = h.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&conn).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			// The (integration_id, nango_connection_id) active-uniqueness index
			// rejected this: another live connection already owns this id.
			writeJSON(w, http.StatusConflict, map[string]string{"error": "connection already exists"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to create connection", "error", err, "org_id", org.ID, "user_id", user.ID, "integration_id", integ.ID)
		logging.CaptureWithFields(r.Context(), fmt.Errorf("create connection: %w", err), map[string]any{
			"org_id":         org.ID.String(),
			"user_id":        user.ID.String(),
			"integration_id": integ.ID.String(),
			"provider":       integ.Provider,
		})
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to create connection"})
		return
	}

	conn.Integration = integ
	logging.FromContext(r.Context()).InfoContext(r.Context(), "connection created", "connection_id", conn.ID, "org_id", org.ID, "user_id", user.ID, "provider", integ.Provider)

	writeJSON(w, http.StatusCreated, h.toConnectionResponse(conn))
}
