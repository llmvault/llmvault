package control

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm/clause"

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

var (
	aliasCharset    = regexp.MustCompile(`^[a-z0-9-]+$`)
	aliasPortPrefix = regexp.MustCompile(`^[0-9]{1,5}-`)
)

// reservedAliases protects operational hostnames and the preview scheme. The
// port-prefix pattern (e.g. "3000-...") is rejected separately because it
// collides with the {port}-{sandbox} preview host grammar.
var reservedAliases = map[string]bool{
	"www":      true,
	"api":      true,
	"admin":    true,
	"preview":  true,
	"static":   true,
	"app":      true,
	"apps":     true,
	"mail":     true,
	"gateway":  true,
	"health":   true,
	"metrics":  true,
	"internal": true,
	"runner":   true,
	"control":  true,
}

func validateAlias(alias string) error {
	if len(alias) < 3 || len(alias) > 63 {
		return fmt.Errorf("alias must be between 3 and 63 characters")
	}
	if !aliasCharset.MatchString(alias) {
		return fmt.Errorf("alias must contain only lowercase letters, digits, and hyphens")
	}
	if strings.HasPrefix(alias, "-") || strings.HasSuffix(alias, "-") {
		return fmt.Errorf("alias must not start or end with a hyphen")
	}
	if aliasPortPrefix.MatchString(alias) {
		return fmt.Errorf("alias must not look like a preview port prefix")
	}
	if reservedAliases[alias] {
		return fmt.Errorf("alias is reserved")
	}
	return nil
}

func normalizeAliasParam(r *http.Request) string {
	return strings.ToLower(strings.TrimSpace(chi.URLParam(r, "alias")))
}

func (s *Server) aliasURL(alias string) string {
	return fmt.Sprintf("https://%s.%s", alias, s.cfg.PreviewBaseDomain)
}

// claimAlias claims a new alias or repoints an existing one to a sandbox+port.
// Repoint is last-write-wins by design (an app redeployed into a fresh sandbox
// keeps its alias by moving the mapping).
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
	now := time.Now().UTC()
	row := model.Alias{Alias: alias, SandboxID: req.SandboxID, Port: req.Port, CreatedAt: now, UpdatedAt: now}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "alias"}},
		DoUpdates: clause.Assignments(map[string]any{
			"sandbox_id": req.SandboxID,
			"port":       req.Port,
			"updated_at": now,
		}),
	}).Create(&row).Error; err != nil {
		httpx.JSON(w, http.StatusInternalServerError, api.ErrorResponse{Error: "failed to persist alias"})
		return
	}
	s.syncAliasRoute(ctx, row)
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
