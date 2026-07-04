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

// aliasRouteSyncAttempts / aliasRouteSyncBackoff bound the retry the claim path
// applies before failing closed: a transient gateway blip should not strand an
// alias URL, but we must not block the claim indefinitely either.
const (
	aliasRouteSyncAttempts = 3
	aliasRouteSyncBackoff  = 100 * time.Millisecond
)

// syncAliasRoute pushes a single alias→(sandbox,port) mapping to the gateway.
// It returns the push error so the claim path can fail closed; the best-effort
// refresh callers (wake/stop/bulk) ignore the return.
func (s *Server) syncAliasRoute(ctx context.Context, a model.Alias) error {
	if s.previewCache == nil {
		return nil
	}
	logger := logging.FromContext(ctx)
	if err := s.previewCache.UpsertAliasRoute(ctx, aliasCacheRouteFor(a)); err != nil {
		logger.ErrorContext(ctx, "alias route sync failed", "alias", a.Alias, "sandbox_id", a.SandboxID, "error", err)
		return err
	}
	logger.InfoContext(ctx, "alias route synced", "alias", a.Alias, "sandbox_id", a.SandboxID)
	return nil
}

// syncAliasRouteWithRetry pushes an alias route with a bounded backoff retry.
// The claim path uses it so a redeployed app never reports a live alias URL the
// gateway cannot resolve: if every attempt fails the final error is returned and
// the caller fails the claim (non-2xx) rather than persisting a dead route.
func (s *Server) syncAliasRouteWithRetry(ctx context.Context, a model.Alias) error {
	if s.previewCache == nil {
		return nil
	}
	var err error
	for attempt := 1; attempt <= aliasRouteSyncAttempts; attempt++ {
		if err = s.syncAliasRoute(ctx, a); err == nil {
			return nil
		}
		if attempt == aliasRouteSyncAttempts {
			break
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(aliasRouteSyncBackoff * time.Duration(attempt)):
		}
	}
	return err
}

// syncSandboxPreviewRoute re-pushes a sandbox's preview route to the gateway by
// id. The claim path calls it so a redeploy into an existing sandbox restores a
// preview route whose gateway TTL lapsed. Best effort: the alias route is the
// claim's hard dependency, and a missing preview route self-heals on the next
// ensure-ready.
func (s *Server) syncSandboxPreviewRoute(ctx context.Context, sandboxID string) {
	if s.previewCache == nil {
		return
	}
	logger := logging.FromContext(ctx)
	var sb model.Sandbox
	if err := s.db.WithContext(ctx).First(&sb, "id = ?", sandboxID).Error; err != nil {
		logger.WarnContext(ctx, "preview route resync load sandbox failed", "sandbox_id", sandboxID, "error", err)
		return
	}
	var runner model.Runner
	if err := s.db.WithContext(ctx).First(&runner, "id = ?", sb.RunnerID).Error; err != nil {
		logger.WarnContext(ctx, "preview route resync load runner failed", "sandbox_id", sandboxID, "runner_id", sb.RunnerID, "error", err)
		return
	}
	s.syncPreviewRoute(ctx, sb, runner, routePorts(ctx, s.db, sb.ID))
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
		_ = s.syncAliasRoute(ctx, a)
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
