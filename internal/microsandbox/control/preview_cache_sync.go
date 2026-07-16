package control

import (
	"context"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func (s *Server) watchPreviewRoutes(ctx context.Context) {
	if s.cfg.PreviewCacheSync <= 0 {
		logging.FromContext(ctx).WarnContext(ctx, "preview route periodic sync disabled because interval is not positive", "interval", s.cfg.PreviewCacheSync)
		return
	}
	ticker := time.NewTicker(s.cfg.PreviewCacheSync)
	defer ticker.Stop()
	for {
		s.bulkSyncPreviewRoutes(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (s *Server) bulkSyncPreviewRoutes(ctx context.Context) {
	if s.previewCache == nil {
		return
	}
	logger := logging.FromContext(ctx)
	var sandboxes []model.Sandbox
	if err := s.db.WithContext(ctx).Find(&sandboxes).Error; err != nil {
		logger.ErrorContext(ctx, "preview route bulk sync sandbox query failed", "error", err)
		return
	}
	var runners []model.Runner
	if err := s.db.WithContext(ctx).Find(&runners).Error; err != nil {
		logger.ErrorContext(ctx, "preview route bulk sync runner query failed", "error", err)
		return
	}
	runnerByID := make(map[string]model.Runner, len(runners))
	for _, runner := range runners {
		runnerByID[runner.ID] = runner
	}
	var allPorts []model.SandboxPort
	if err := s.db.WithContext(ctx).Order("sandbox_id asc, guest_port asc").Find(&allPorts).Error; err != nil {
		logger.ErrorContext(ctx, "preview route bulk sync ports query failed", "error", err)
		return
	}
	portsBySandbox := map[string][]model.SandboxPort{}
	for _, port := range allPorts {
		portsBySandbox[port.SandboxID] = append(portsBySandbox[port.SandboxID], port)
	}
	existingSandboxes := make(map[string]bool, len(sandboxes))
	routes := make([]previewCacheRoute, 0, len(sandboxes))
	for _, sb := range sandboxes {
		existingSandboxes[sb.ID] = true
		runner, ok := runnerByID[sb.RunnerID]
		if !ok {
			logger.WarnContext(ctx, "preview route bulk sync skipped sandbox without runner", "sandbox_id", sb.ID, "runner_id", sb.RunnerID)
			continue
		}
		ports := portsBySandbox[sb.ID]
		if len(ports) == 0 {
			continue
		}
		route, err := previewCacheRouteFor(sb, runner, ports)
		if err != nil {
			logger.WarnContext(ctx, "preview route bulk sync skipped sandbox", "sandbox_id", sb.ID, "runner_id", runner.ID, "error", err)
			continue
		}
		routes = append(routes, route)
	}
	s.bulkSyncAliasRoutes(ctx, existingSandboxes)
	if len(routes) == 0 {
		return
	}
	if err := s.previewCache.BulkUpsertRoutes(ctx, routes); err != nil {
		logger.ErrorContext(ctx, "preview route bulk sync failed", "routes", len(routes), "error", err)
		return
	}
	logger.InfoContext(ctx, "preview routes bulk synced", "routes", len(routes))
}
