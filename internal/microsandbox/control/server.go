package control

import (
	"context"
	"net/http"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/keyedlock"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/httpx"
	"github.com/usehivy/hivy/internal/microsandbox/model"
	"github.com/usehivy/hivy/internal/observability/correlation"
	obsmetrics "github.com/usehivy/hivy/internal/observability/metrics"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
)

type Server struct {
	db             *gorm.DB
	cfg            config.Config
	client         *RunnerClient
	previewCache   *PreviewCacheClient
	lifecycleLocks keyedlock.Locker
}

func NewServer(ctx context.Context, db *gorm.DB, cfg config.Config) *Server {
	s := &Server{
		db:           db,
		cfg:          cfg,
		client:       NewRunnerClient(cfg.RunnerAPIToken),
		previewCache: NewPreviewCacheClient(ctx, cfg),
	}
	goroutine.Go(ctx, func(ctx context.Context) {
		s.watchRunners(ctx)
	})
	if s.previewCache != nil {
		goroutine.Go(ctx, func(ctx context.Context) {
			s.watchPreviewRoutes(ctx)
		})
	}
	return s
}

func (s *Server) Routes() http.Handler {
	r := chi.NewRouter()
	r.Use(obsmetrics.HTTPMiddleware("microsandbox-control"))
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
	r.Handle("/metrics", obsmetrics.HandlerWith(newFleetCollector(s.db)))

	r.Post("/v1/runners/register", s.registerRunner)
	r.Post("/v1/runners/{runnerID}/heartbeat", s.runnerHeartbeat)
	r.Post("/v1/sandboxes/activity/bulk", s.sandboxActivityBulk)
	r.Post("/v1/sandboxes/{sandboxID}/activity", s.sandboxActivity)

	r.Group(func(r chi.Router) {
		r.Use(s.requireAPI)
		r.Get("/v1/runners", s.listRunners)
		r.Get("/v1/orgs/{orgID}/preview-password", s.getOrgPreviewPassword)
		r.Put("/v1/orgs/{orgID}/preview-password", s.updateOrgPreviewPassword)
		r.Get("/v1/sandboxes", s.listSandboxes)
		r.Get("/v1/sandboxes/states", s.listSandboxStates)
		r.Post("/v1/sandboxes", s.createSandbox)
		r.Get("/v1/sandboxes/{sandboxID}", s.getSandbox)
		r.Get("/v1/sandboxes/{sandboxID}/route", s.getSandboxRoute)
		r.Post("/v1/sandboxes/{sandboxID}/ensure-ready", s.ensureSandboxReady)
		r.Patch("/v1/sandboxes/{sandboxID}/policy", s.updateSandboxPolicy)
		r.Post("/v1/sandboxes/{sandboxID}/runtime-endpoints", s.createRuntimeEndpoint)
		r.Post("/v1/sandboxes/{sandboxID}/start", s.startSandbox)
		r.Post("/v1/sandboxes/{sandboxID}/stop", s.stopSandbox)
		r.Delete("/v1/sandboxes/{sandboxID}", s.deleteSandbox)
		r.Post("/v1/sandboxes/{sandboxID}/exec", s.execSandbox)
		r.Get("/v1/sandboxes/{sandboxID}/logs", s.logsSandbox)
		r.Post("/v1/templates", s.createTemplate)
		r.Get("/v1/templates/{templateID}", s.getTemplate)
		r.Delete("/v1/templates/{templateID}", s.deleteTemplate)
		r.Put("/v1/aliases/{alias}", s.claimAlias)
		r.Get("/v1/aliases/{alias}", s.getAlias)
		r.Delete("/v1/aliases/{alias}", s.deleteAlias)
	})

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

func (s *Server) watchRunners(ctx context.Context) {
	ticker := time.NewTicker(s.cfg.RunnerCheckInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
		cutoff := time.Now().Add(-s.cfg.RunnerUnhealthyAfter)
		var runners []model.Runner
		if err := s.db.WithContext(ctx).Where("last_heartbeat_at IS NULL OR last_heartbeat_at < ?", cutoff).Find(&runners).Error; err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "runner health query failed", "error", err)
			continue
		}
		for _, runner := range runners {
			s.db.WithContext(ctx).Model(&runner).Update("status", model.RunnerStatusUnhealthy)
			err := &runnerUnhealthyError{RunnerID: runner.ID, Name: runner.Name, LastHeartbeatAt: runner.LastHeartbeatAt}
			sentry.CaptureException(err)
			logging.FromContext(ctx).ErrorContext(ctx, "runner heartbeat overdue", "runner_id", runner.ID, "name", runner.Name, "last_heartbeat_at", runner.LastHeartbeatAt)
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
