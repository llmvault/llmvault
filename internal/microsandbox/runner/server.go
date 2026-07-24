package runner

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/observability/correlation"
	obsmetrics "github.com/usehivy/hivy/internal/observability/metrics"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

type Server struct {
	cfg                config.Config
	backend            Backend
	runnerID           string
	token              string
	tokenMu            sync.RWMutex
	http               *http.Client
	pressure           *hostPressureSampler
	startingOperations atomic.Int64
	logForwardURL      *url.URL
	logStreamsMu       sync.Mutex
	logStreams         map[string]int
	logAccepted        atomic.Uint64
	logRejected        atomic.Uint64
	logUpstreamErrors  atomic.Uint64
}

func NewServer(ctx context.Context, cfg config.Config) (*Server, error) {
	var backend Backend
	switch cfg.RunnerBackend {
	case "mock":
		backend = NewMockBackend()
	default:
		msb, err := NewMicrosandboxBackend(ctx, cfg)
		if err != nil {
			return nil, err
		}
		backend = msb
	}
	var logForwardURL *url.URL
	if raw := strings.TrimSpace(cfg.RunnerLogForwardURL); raw != "" {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Scheme == "" || parsed.Host == "" {
			return nil, fmt.Errorf("invalid HIVY_MICROSANDBOX_RUNNER_LOG_FORWARD_URL")
		}
		logForwardURL = parsed
	}
	return &Server{
		cfg: cfg, backend: backend, http: &http.Client{Timeout: 2 * time.Minute},
		pressure: newHostPressureSampler(), logForwardURL: logForwardURL,
		logStreams: map[string]int{},
	}, nil
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(obsmetrics.HTTPMiddleware("microsandbox-runner"))
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(sentryobs.Middleware())
	r.Use(correlation.Middleware)
	r.Use(sentryobs.Capture5xxResponses())
	r.Use(middleware.Recoverer)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	r.Handle("/metrics", obsmetrics.Handler())
	r.Group(func(r chi.Router) {
		r.Use(s.requireControl)
		r.Post("/v1/sandboxes", s.createSandbox)
		r.Post("/v1/sandboxes/{sandboxID}/start", s.startSandbox)
		r.Post("/v1/sandboxes/{sandboxID}/stop", s.stopSandbox)
		r.Post("/v1/sandboxes/{sandboxID}/ensure-ready", s.ensureSandboxReady)
		r.Get("/v1/sandboxes/{sandboxID}/connections", s.sandboxConnections)
		r.Post("/v1/sandboxes/{sandboxID}/delete", s.deleteSandbox)
		r.Post("/v1/sandboxes/{sandboxID}/exec", s.execSandbox)
		r.Get("/v1/sandboxes/{sandboxID}/logs", s.logsSandbox)
		r.Post("/v1/templates", s.createTemplate)
		r.Handle("/proxy/{sandboxID}/{port}/*", s.proxySandbox())
	})
	return r
}

func (s *Server) requireControl(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.RunnerAPIToken == "" || httpx.Bearer(r) != s.cfg.RunnerAPIToken {
			httpx.JSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) RegisterAndHeartbeat(ctx context.Context) {
	logger := logging.FromContext(ctx)
	for {
		if err := s.register(ctx); err != nil {
			logger.ErrorContext(ctx, "runner registration failed", "error", err)
			sleep(ctx, 5*time.Second)
			continue
		}
		break
	}
	report, err := s.backend.Reconcile(ctx)
	if err != nil {
		logger.ErrorContext(ctx, "runner sandbox reconciliation failed", "error", err)
	} else {
		logger.InfoContext(ctx, "runner sandbox reconciliation complete", "sandboxes", report.Sandboxes, "ports", report.Ports, "skipped", report.Skipped)
	}
	ticker := time.NewTicker(s.cfg.HeartbeatInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := s.heartbeat(ctx); err != nil {
				logger.ErrorContext(ctx, "runner heartbeat failed", "error", err)
			}
		}
	}
}

func (s *Server) register(ctx context.Context) error {
	if s.cfg.ControlURL == "" || s.cfg.RunnerJoinSecret == "" || s.cfg.RunnerPublicURL == "" || s.cfg.RunnerPreviewBaseURL == "" {
		return fmt.Errorf("HIVY_MICROSANDBOX_CONTROL_URL, HIVY_MICROSANDBOX_RUNNER_JOIN_SECRET, HIVY_MICROSANDBOX_RUNNER_PUBLIC_URL, and HIVY_MICROSANDBOX_RUNNER_PREVIEW_BASE_URL are required")
	}
	body := map[string]any{
		"name":             s.cfg.RunnerName,
		"api_url":          s.cfg.RunnerPublicURL,
		"preview_base_url": s.cfg.RunnerPreviewBaseURL,
		"capacity": map[string]any{
			"cpu": s.cfg.RunnerTotalCPU, "memory_mb": s.cfg.RunnerTotalMemoryMB,
			"disk_gb": s.cfg.RunnerTotalDiskGB, "cpu_overcommit": s.cfg.RunnerCPUOvercommit,
			"memory_overcommit": s.cfg.RunnerMemoryOvercommit, "disk_overcommit": s.cfg.RunnerDiskOvercommit,
		},
	}
	var out struct {
		RunnerID                 string `json:"runner_id"`
		RunnerToken              string `json:"runner_token"`
		HeartbeatIntervalSeconds int    `json:"heartbeat_interval_seconds"`
	}
	if err := s.postControl(ctx, "/v1/runners/register", s.cfg.RunnerJoinSecret, body, &out); err != nil {
		return err
	}
	s.tokenMu.Lock()
	s.runnerID = out.RunnerID
	s.token = out.RunnerToken
	s.tokenMu.Unlock()
	logging.FromContext(ctx).InfoContext(ctx, "runner registered", "runner_id", out.RunnerID)
	return nil
}

func (s *Server) heartbeat(ctx context.Context) error {
	s.tokenMu.RLock()
	runnerID, token := s.runnerID, s.token
	s.tokenMu.RUnlock()
	if runnerID == "" || token == "" {
		return s.register(ctx)
	}
	status, err := s.backend.Status(ctx)
	if err != nil {
		return err
	}
	status["pressure"] = s.pressure.Sample(int(s.startingOperations.Load()))
	return s.postControl(ctx, "/v1/runners/"+runnerID+"/heartbeat", token, status, nil)
}

func (s *Server) trackStartingOperation() func() {
	s.startingOperations.Add(1)
	return func() { s.startingOperations.Add(-1) }
}

func (s *Server) postControl(ctx context.Context, path, token string, in, out any) error {
	raw, err := json.Marshal(in)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.cfg.ControlURL, "/")+path, bytes.NewReader(raw))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		return fmt.Errorf("control returned %d", resp.StatusCode)
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func sleep(ctx context.Context, d time.Duration) {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-ctx.Done():
	case <-timer.C:
	}
}
