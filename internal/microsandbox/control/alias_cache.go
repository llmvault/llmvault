package control

import (
	"context"
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

// aliasCacheRoute is the alias→(sandbox,port) mapping pushed to the gateway
// store. It is keyed distinctly from preview routes; the gateway resolves an
// alias host to (sandbox_id, port) and then reuses the identical
// ensure-ready/wake/lease/activity machinery keyed on the sandbox, so
// wake-on-request works unchanged.
type aliasCacheRoute struct {
	Alias     string `json:"alias"`
	SandboxID string `json:"sandbox_id"`
	Port      int    `json:"port"`
	UpdatedAt int64  `json:"updated_at"`
}

func aliasCacheRouteFor(a model.Alias) aliasCacheRoute {
	return aliasCacheRoute{
		Alias:     a.Alias,
		SandboxID: a.SandboxID,
		Port:      a.Port,
		UpdatedAt: time.Now().UTC().Unix(),
	}
}

func (c *PreviewCacheClient) UpsertAliasRoute(ctx context.Context, route aliasCacheRoute) error {
	return c.do(ctx, http.MethodPut, "/v1/aliases/"+route.Alias, route, nil)
}

func (c *PreviewCacheClient) BulkUpsertAliasRoutes(ctx context.Context, routes []aliasCacheRoute) error {
	if len(routes) == 0 {
		return nil
	}
	return c.do(ctx, http.MethodPost, "/v1/aliases/bulk", map[string]any{"aliases": routes}, nil)
}

func (c *PreviewCacheClient) DeleteAliasRoute(ctx context.Context, alias string) error {
	return c.do(ctx, http.MethodDelete, "/v1/aliases/"+alias, nil, nil)
}

func (s *Server) syncAliasRoute(ctx context.Context, a model.Alias) {
	if s.previewCache == nil {
		return
	}
	logger := logging.FromContext(ctx)
	if err := s.previewCache.UpsertAliasRoute(ctx, aliasCacheRouteFor(a)); err != nil {
		logger.ErrorContext(ctx, "alias route sync failed", "alias", a.Alias, "sandbox_id", a.SandboxID, "error", err)
		return
	}
	logger.InfoContext(ctx, "alias route synced", "alias", a.Alias, "sandbox_id", a.SandboxID)
}

func (s *Server) deleteAliasRoute(ctx context.Context, alias string) {
	if s.previewCache == nil {
		return
	}
	logger := logging.FromContext(ctx)
	if err := s.previewCache.DeleteAliasRoute(ctx, alias); err != nil {
		logger.ErrorContext(ctx, "alias route delete failed", "alias", alias, "error", err)
		return
	}
	logger.InfoContext(ctx, "alias route deleted", "alias", alias)
}

func (s *Server) aliasesForSandbox(ctx context.Context, sandboxID string) []model.Alias {
	var aliases []model.Alias
	if err := s.db.WithContext(ctx).Find(&aliases, "sandbox_id = ?", sandboxID).Error; err != nil {
		logging.FromContext(ctx).WarnContext(ctx, "load sandbox aliases failed", "sandbox_id", sandboxID, "error", err)
	}
	return aliases
}

// syncSandboxAliasRoutes refreshes every alias route for a sandbox alongside its
// preview route (on create/wake/stop), so a repointed or expired alias entry is
// restored whenever the sandbox route is refreshed.
func (s *Server) syncSandboxAliasRoutes(ctx context.Context, sandboxID string) {
	if s.previewCache == nil {
		return
	}
	for _, a := range s.aliasesForSandbox(ctx, sandboxID) {
		s.syncAliasRoute(ctx, a)
	}
}

// deleteSandboxAliasRoutes removes the gateway alias entries for a deleted
// sandbox. The alias→sandbox rows themselves are intentionally kept in the
// control-plane DB (dead-sandbox mapping survives), so a redeploy that repoints
// the alias to a fresh sandbox restores routing; until then lookups fail.
func (s *Server) deleteSandboxAliasRoutes(ctx context.Context, sandboxID string) {
	if s.previewCache == nil {
		return
	}
	for _, a := range s.aliasesForSandbox(ctx, sandboxID) {
		s.deleteAliasRoute(ctx, a.Alias)
	}
}

func (s *Server) bulkSyncAliasRoutes(ctx context.Context, existingSandboxes map[string]bool) {
	if s.previewCache == nil {
		return
	}
	logger := logging.FromContext(ctx)
	var aliases []model.Alias
	if err := s.db.WithContext(ctx).Find(&aliases).Error; err != nil {
		logger.ErrorContext(ctx, "alias route bulk sync query failed", "error", err)
		return
	}
	routes := make([]aliasCacheRoute, 0, len(aliases))
	for _, a := range aliases {
		if !existingSandboxes[a.SandboxID] {
			// Dead-sandbox alias: keep the DB mapping but do not route it.
			continue
		}
		routes = append(routes, aliasCacheRouteFor(a))
	}
	if len(routes) == 0 {
		return
	}
	if err := s.previewCache.BulkUpsertAliasRoutes(ctx, routes); err != nil {
		logger.ErrorContext(ctx, "alias route bulk sync failed", "routes", len(routes), "error", err)
		return
	}
	logger.InfoContext(ctx, "alias routes bulk synced", "routes", len(routes))
}
