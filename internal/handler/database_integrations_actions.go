package handler

import (
	"encoding/json"
	"fmt"
	"net/http"

	dbi "github.com/usehivy/hivy/internal/databaseintegration"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// Test handles POST /v1/database-integrations/{id}/test.
// @Summary Test database integration
// @Description Verifies that stored database credentials can connect.
// @Tags database-integrations
// @Produce json
// @Param id path string true "Database integration ID"
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/database-integrations/{id}/test [post]
func (h *DatabaseIntegrationHandler) Test(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.loadOrgConnection(w, r)
	if !ok {
		return
	}
	dsn, ok := h.decryptDSN(w, r, conn)
	if !ok {
		return
	}
	if err := testDatabaseConnection(r.Context(), conn.Provider, dsn); err != nil {
		logging.Capture(r.Context(), fmt.Errorf("database integration test failed: %w", err))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "database connection test failed"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"ok": true})
}

// Introspect handles POST /v1/database-integrations/{id}/introspect.
// @Summary Introspect database integration
// @Description Introspects and stores the visible database schema snapshot.
// @Tags database-integrations
// @Produce json
// @Param id path string true "Database integration ID"
// @Success 200 {object} databaseConnectionResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Security BearerAuth
// @Router /v1/database-integrations/{id}/introspect [post]
func (h *DatabaseIntegrationHandler) Introspect(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.loadOrgConnection(w, r)
	if !ok {
		return
	}
	dsn, ok := h.decryptDSN(w, r, conn)
	if !ok {
		return
	}
	snapshot, err := introspectDatabase(r.Context(), conn.Provider, dsn)
	if err != nil {
		logging.Capture(r.Context(), fmt.Errorf("database integration introspect failed: %w", err))
		writeJSON(w, http.StatusBadGateway, map[string]string{"error": "database introspection failed"})
		return
	}
	raw, _ := json.Marshal(snapshot)
	if err := h.db.WithContext(r.Context()).Model(&conn).Update("schema_snapshot", model.RawJSON(raw)).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save schema snapshot"})
		return
	}
	conn.SchemaSnapshot = model.RawJSON(raw)
	writeJSON(w, http.StatusOK, databaseConnectionToResponse(conn))
}

// UpdatePolicy handles PUT /v1/database-integrations/{id}/policy.
// @Summary Update database access policy
// @Description Saves table, collection, field mask, and result limit policy for a database integration.
// @Tags database-integrations
// @Accept json
// @Produce json
// @Param id path string true "Database integration ID"
// @Param body body databaseintegration.Policy true "Access policy"
// @Success 200 {object} databaseConnectionResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/database-integrations/{id}/policy [put]
func (h *DatabaseIntegrationHandler) UpdatePolicy(w http.ResponseWriter, r *http.Request) {
	conn, ok := h.loadOrgConnection(w, r)
	if !ok {
		return
	}
	var policy dbi.Policy
	if err := json.NewDecoder(r.Body).Decode(&policy); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid access policy"})
		return
	}
	conn.AccessPolicy = dbi.PolicyToJSON(policy)
	if err := h.db.WithContext(r.Context()).Model(&conn).Update("access_policy", conn.AccessPolicy).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to save access policy"})
		return
	}
	writeJSON(w, http.StatusOK, databaseConnectionToResponse(conn))
}

func (h *DatabaseIntegrationHandler) decryptDSN(w http.ResponseWriter, r *http.Request, conn model.DatabaseConnection) (string, bool) {
	dsn, err := dbi.DecryptSecret(r.Context(), h.kms, conn.EncryptedDSN, conn.WrappedDEK)
	if err != nil {
		logging.Capture(r.Context(), fmt.Errorf("database integration decrypt: %w", err))
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to decrypt database credentials"})
		return "", false
	}
	return dsn, true
}
