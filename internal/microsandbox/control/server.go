package control

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

type Server struct {
	db           *gorm.DB
	cfg          config.Config
	client       *RunnerClient
	previewCache *PreviewCacheClient
}

func NewServer(db *gorm.DB, cfg config.Config) *Server {
	s := &Server{
		db:           db,
		cfg:          cfg,
		client:       NewRunnerClient(cfg.RunnerAPIToken),
		previewCache: NewPreviewCacheClient(cfg),
	}
	go s.watchRunners()
	if s.previewCache != nil {
		go s.watchPreviewRoutes()
	}
	return s
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(middleware.RequestID)
	r.Use(middleware.RealIP)
	r.Use(middleware.Recoverer)
	r.Get("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})

	r.Post("/v1/runners/register", s.registerRunner)
	r.Post("/v1/runners/{runnerID}/heartbeat", s.runnerHeartbeat)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAPI)
		r.Get("/v1/runners", s.listRunners)
		r.Get("/v1/orgs/{orgID}/preview-password", s.getOrgPreviewPassword)
		r.Put("/v1/orgs/{orgID}/preview-password", s.updateOrgPreviewPassword)
		r.Get("/v1/sandboxes", s.listSandboxes)
		r.Post("/v1/sandboxes", s.createSandbox)
		r.Get("/v1/sandboxes/{sandboxID}", s.getSandbox)
		r.Post("/v1/sandboxes/{sandboxID}/runtime-endpoints", s.createRuntimeEndpoint)
		r.Post("/v1/sandboxes/{sandboxID}/start", s.startSandbox)
		r.Post("/v1/sandboxes/{sandboxID}/stop", s.stopSandbox)
		r.Delete("/v1/sandboxes/{sandboxID}", s.deleteSandbox)
		r.Post("/v1/sandboxes/{sandboxID}/exec", s.execSandbox)
		r.Get("/v1/sandboxes/{sandboxID}/logs", s.logsSandbox)
		r.Post("/v1/preview-sessions", s.createPreviewSession)
		r.Post("/v1/snapshots", s.createSnapshot)
		r.Get("/v1/snapshots/{snapshotID}", s.getSnapshot)
	})

	r.Get("/preview/login", s.previewLoginForm)
	r.Post("/preview/login", s.previewLoginPost)
	r.Handle("/*", s.previewProxy())
	return r
}

func (s *Server) requireAPI(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if s.cfg.APIToken == "" || httpx.Bearer(r) != s.cfg.APIToken {
			httpx.JSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
			return
		}
		next.ServeHTTP(w, r)
	})
}

func (s *Server) watchRunners() {
	ticker := time.NewTicker(s.cfg.RunnerCheckInterval)
	defer ticker.Stop()
	for range ticker.C {
		cutoff := time.Now().Add(-s.cfg.RunnerUnhealthyAfter)
		var runners []model.Runner
		if err := s.db.Where("last_heartbeat_at IS NULL OR last_heartbeat_at < ?", cutoff).Find(&runners).Error; err != nil {
			slog.Error("runner health query failed", "error", err)
			continue
		}
		for _, runner := range runners {
			s.db.Model(&runner).Update("status", model.RunnerStatusUnhealthy)
			err := &runnerUnhealthyError{RunnerID: runner.ID, Name: runner.Name, LastHeartbeatAt: runner.LastHeartbeatAt}
			sentry.CaptureException(err)
			slog.Error("runner heartbeat overdue", "runner_id", runner.ID, "name", runner.Name, "last_heartbeat_at", runner.LastHeartbeatAt)
		}
	}
}

type runnerUnhealthyError struct {
	RunnerID        string
	Name            string
	LastHeartbeatAt *time.Time
}

func (e *runnerUnhealthyError) Error() string {
	return "runner heartbeat overdue: " + e.RunnerID + " (" + e.Name + ")"
}
