package control

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/microsandbox/alias"
	"github.com/usehivy/hivy/internal/microsandbox/api"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

type claimAliasRequest struct {
	SandboxID string `json:"sandbox_id"`
	Port      int    `json:"port"`
}

type aliasResponse struct {
	Alias     string `json:"alias"`
	URL       string `json:"url"`
	SandboxID string `json:"sandbox_id"`
	Port      int    `json:"port"`
}

// validateAlias delegates to the shared alias ruleset (internal/microsandbox/alias)
// so the control plane and the apps slug derivation stay in lockstep.
func validateAlias(a string) error {
	return alias.Validate(a)
}

func normalizeAliasParam(r *http.Request) string {
	return alias.Normalize(chi.URLParam(r, "alias"))
}

func (s *Server) aliasURL(alias string) string {
	return fmt.Sprintf("https://%s.%s", alias, s.cfg.PreviewBaseDomain)
}

// claimAlias claims a new alias or repoints an existing one to a sandbox+port.
// Repoint is last-write-wins WITHIN an org (an app redeployed into a fresh
// sandbox keeps its alias by moving the mapping). Cross-org repoints are
// rejected: an alias is owned by the org of the sandbox that first claimed it,
// and another org cannot repoint {slug}.{PreviewBaseDomain} at its own sandbox
// (hostname/content takeover). Ownership is derived from the sandbox, never
// trusted from the request.
func (s *Server) claimAlias(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	alias := normalizeAliasParam(r)
	if err := validateAlias(alias); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: err.Error()})
		return
	}
	var req claimAliasRequest
	if err := httpx.Decode(r, &req); err != nil {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "invalid request body"})
		return
	}
	req.SandboxID = strings.TrimSpace(req.SandboxID)
	if req.SandboxID == "" {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "sandbox_id is required"})
		return
	}
	if req.Port <= 0 || req.Port > 65535 {
		httpx.JSON(w, http.StatusBadRequest, api.ErrorResponse{Error: "valid port is required"})
		return
	}
	var sb model.Sandbox
	if err := s.db.WithContext(ctx).First(&sb, "id = ?", req.SandboxID).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "sandbox not found"})
		return
	}
	// The claim is owned by the sandbox's org, and an existing alias may only be
	// repointed by its owning org. This closes the cross-org hostname takeover:
	// Org B claiming Org A's slug is a 409, not a silent last-write-wins repoint.
	var existing model.Alias
	switch err := s.db.WithContext(ctx).First(&existing, "alias = ?", alias).Error; {
	case err == nil:
		if existing.OrgID != "" && existing.OrgID != sb.OrgID {
			httpx.JSON(w, http.StatusConflict, api.ErrorResponse{Error: "alias is claimed by another organization"})
			return
		}
	case errors.Is(err, gorm.ErrRecordNotFound):
		// fresh claim
	default:
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to look up alias"})
		return
	}
	now := time.Now().UTC()
	row := model.Alias{Alias: alias, OrgID: sb.OrgID, SandboxID: req.SandboxID, Port: req.Port, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "alias"}},
		// org_id is intentionally NOT reassigned on conflict: an alias keeps its
		// original owner. The cross-org guard above already rejects a foreign
		// claimant, so this only ever repoints within the owning org.
		DoUpdates: clause.Assignments(map[string]any{
			"sandbox_id": req.SandboxID,
			"port":       req.Port,
			"updated_at": now,
		}),
	}).Create(&row).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to persist alias"})
		return
	}
	// Fail the claim closed when the gateway never gets the mapping: returning
	// 200 + URL here (the old behavior) reported a dead alias as success and
	// stranded redeploys behind "invalid preview host". The caller marks the
	// deploy failed on this non-2xx instead.
	if err := s.syncAliasRouteWithRetry(ctx, row); err != nil {
		httpx.JSON(w, http.StatusBadGateway, api.ErrorResponse{Error: "failed to propagate alias route to gateway"})
		return
	}
	// Re-push the sandbox's preview route too, so a redeploy into an existing
	// sandbox restores a preview route whose gateway TTL lapsed (best effort).
	s.syncSandboxPreviewRoute(ctx, row.SandboxID)
	httpx.JSON(w, http.StatusOK, aliasResponse{
		Alias:     alias,
		URL:       s.aliasURL(alias),
		SandboxID: req.SandboxID,
		Port:      req.Port,
	})
}

func (s *Server) getAlias(w http.ResponseWriter, r *http.Request) {
	alias := normalizeAliasParam(r)
	var row model.Alias
	if err := s.db.WithContext(r.Context()).First(&row, "alias = ?", alias).Error; err != nil {
		httpx.JSON(w, http.StatusNotFound, api.ErrorResponse{Error: "alias not found"})
		return
	}
	httpx.JSON(w, http.StatusOK, aliasResponse{
		Alias:     row.Alias,
		URL:       s.aliasURL(row.Alias),
		SandboxID: row.SandboxID,
		Port:      row.Port,
	})
}

// deleteAlias removes an alias mapping. Deleting an unknown alias is a success
// (204) so callers can idempotently release.
func (s *Server) deleteAlias(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	alias := normalizeAliasParam(r)
	var row model.Alias
	found := s.db.WithContext(ctx).First(&row, "alias = ?", alias).Error == nil
	if err := s.db.WithContext(ctx).Where("alias = ?", alias).Delete(&model.Alias{}).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to delete alias"})
		return
	}
	if found {
		s.deleteAliasRoute(ctx, alias)
	}
	w.WriteHeader(http.StatusNoContent)
}
