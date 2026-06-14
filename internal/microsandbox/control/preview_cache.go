package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

type PreviewCacheClient struct {
	baseURL string
	token   string
	client  *http.Client
}

type previewCacheRoute struct {
	SandboxID string            `json:"sandbox_id"`
	Status    string            `json:"status"`
	Upstreams map[string]string `json:"upstreams"`
}

func NewPreviewCacheClient(cfg config.Config) *PreviewCacheClient {
	if cfg.PreviewCacheURL == "" && cfg.PreviewCacheToken == "" {
		return nil
	}
	if cfg.PreviewCacheURL == "" || cfg.PreviewCacheToken == "" {
		slog.Warn("preview cache partially configured; route sync disabled",
			"has_url", cfg.PreviewCacheURL != "",
			"has_token", cfg.PreviewCacheToken != "",
		)
		return nil
	}
	return &PreviewCacheClient{
		baseURL: strings.TrimRight(cfg.PreviewCacheURL, "/"),
		token:   cfg.PreviewCacheToken,
		client:  &http.Client{Timeout: 10 * time.Second},
	}
}

func (c *PreviewCacheClient) UpsertRoute(ctx context.Context, route previewCacheRoute) error {
	return c.do(ctx, http.MethodPut, "/v1/routes/"+route.SandboxID, route, nil)
}

func (c *PreviewCacheClient) BulkUpsertRoutes(ctx context.Context, routes []previewCacheRoute) error {
	if len(routes) == 0 {
		return nil
	}
	return c.do(ctx, http.MethodPost, "/v1/routes/bulk", map[string]any{"routes": routes}, nil)
}

func (c *PreviewCacheClient) DeleteRoute(ctx context.Context, sandboxID string) error {
	return c.do(ctx, http.MethodDelete, "/v1/routes/"+sandboxID, nil, nil)
}

func (c *PreviewCacheClient) do(ctx context.Context, method, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("preview cache returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func previewCacheRouteFor(sb model.Sandbox, runner model.Runner, ports []model.SandboxPort) (previewCacheRoute, error) {
	if strings.TrimSpace(runner.PreviewBaseURL) == "" {
		return previewCacheRoute{}, fmt.Errorf("runner %s has no preview_base_url", runner.ID)
	}
	upstreams := make(map[string]string, len(ports))
	for _, port := range ports {
		if port.GuestPort <= 0 || port.HostPort <= 0 {
			continue
		}
		upstream, err := previewUpstream(runner.PreviewBaseURL, port.HostPort)
		if err != nil {
			return previewCacheRoute{}, err
		}
		upstreams[strconv.Itoa(port.GuestPort)] = upstream
	}
	if len(upstreams) == 0 {
		return previewCacheRoute{}, fmt.Errorf("sandbox %s has no direct preview upstreams", sb.ID)
	}
	return previewCacheRoute{SandboxID: sb.ID, Status: sb.Status, Upstreams: upstreams}, nil
}

func previewUpstream(baseURL string, hostPort int) (string, error) {
	baseURL = strings.TrimSpace(strings.TrimRight(baseURL, "/"))
	if baseURL == "" {
		return "", fmt.Errorf("preview base url is required")
	}
	if !strings.Contains(baseURL, "://") {
		baseURL = "http://" + baseURL
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		return "", err
	}
	host := parsed.Hostname()
	if host == "" {
		return "", fmt.Errorf("preview base url has no host: %s", baseURL)
	}
	parsed.Host = net.JoinHostPort(host, strconv.Itoa(hostPort))
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

func (s *Server) syncPreviewRoute(ctx context.Context, sb model.Sandbox, runner model.Runner, ports []model.SandboxPort) {
	if s.previewCache == nil {
		return
	}
	if len(ports) == 0 {
		slog.Warn("preview route sync skipped because sandbox has no ports", "sandbox_id", sb.ID)
		return
	}
	route, err := previewCacheRouteFor(sb, runner, ports)
	if err != nil {
		slog.Error("preview route build failed", "sandbox_id", sb.ID, "runner_id", runner.ID, "error", err)
		return
	}
	if err := s.previewCache.UpsertRoute(ctx, route); err != nil {
		slog.Error("preview route sync failed", "sandbox_id", sb.ID, "error", err)
		return
	}
	slog.Info("preview route synced", "sandbox_id", sb.ID, "ports", len(ports))
}

func (s *Server) deletePreviewRoute(ctx context.Context, sandboxID string) {
	if s.previewCache == nil {
		return
	}
	if err := s.previewCache.DeleteRoute(ctx, sandboxID); err != nil {
		slog.Error("preview route delete failed", "sandbox_id", sandboxID, "error", err)
		return
	}
	slog.Info("preview route deleted", "sandbox_id", sandboxID)
}

func (s *Server) watchPreviewRoutes(ctx context.Context) {
	if s.cfg.PreviewCacheSync <= 0 {
		slog.Warn("preview route periodic sync disabled because interval is not positive", "interval", s.cfg.PreviewCacheSync)
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
	var sandboxes []model.Sandbox
	if err := s.db.WithContext(ctx).Find(&sandboxes).Error; err != nil {
		slog.Error("preview route bulk sync sandbox query failed", "error", err)
		return
	}
	routes := make([]previewCacheRoute, 0, len(sandboxes))
	for _, sb := range sandboxes {
		var runner model.Runner
		if err := s.db.WithContext(ctx).First(&runner, "id = ?", sb.RunnerID).Error; err != nil {
			slog.Warn("preview route bulk sync skipped sandbox without runner", "sandbox_id", sb.ID, "runner_id", sb.RunnerID)
			continue
		}
		var ports []model.SandboxPort
		if err := s.db.WithContext(ctx).Order("guest_port asc").Find(&ports, "sandbox_id = ?", sb.ID).Error; err != nil {
			slog.Warn("preview route bulk sync skipped sandbox without ports", "sandbox_id", sb.ID, "error", err)
			continue
		}
		if len(ports) == 0 {
			continue
		}
		route, err := previewCacheRouteFor(sb, runner, ports)
		if err != nil {
			slog.Warn("preview route bulk sync skipped sandbox", "sandbox_id", sb.ID, "runner_id", runner.ID, "error", err)
			continue
		}
		routes = append(routes, route)
	}
	if len(routes) == 0 {
		return
	}
	if err := s.previewCache.BulkUpsertRoutes(ctx, routes); err != nil {
		slog.Error("preview route bulk sync failed", "routes", len(routes), "error", err)
		return
	}
	slog.Info("preview routes bulk synced", "routes", len(routes))
}
