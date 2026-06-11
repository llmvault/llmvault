package main

import (
	"context"
	"crypto/subtle"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/hibiken/asynqmon"

	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/model"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
	"github.com/usehivy/hivy/internal/precontext"
	// Blank import populates interfaces.Registry via init().
	_ "github.com/usehivy/hivy/internal/rag/connectors"
	ragscheduler "github.com/usehivy/hivy/internal/rag/scheduler"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
	"github.com/usehivy/hivy/internal/skills"
	"github.com/usehivy/hivy/internal/tasks"
)

func runWork(ctx context.Context, deps *bootstrap.Deps) error {
	cfg := deps.Config

	redisOpt, err := cfg.AsynqRedisOpt()
	if err != nil {
		return fmt.Errorf("worker: %w", err)
	}

	var workerSender email.Sender = &email.LogSender{}
	if cfg.ResendAPIKey != "" {
		workerSender = email.NewResendSender(cfg.ResendAPIKey, cfg.ResendFrom)
	} else {
		slog.Warn("HIVY_RESEND_API_KEY not set — emails will be logged only")
	}

	enqueuer := enqueue.NewClient(redisOpt)
	if deps.Orchestrator != nil {
		deps.Orchestrator.SetWarmPoolReconciler(func(ctx context.Context, providerID, mode string) error {
			return tasks.EnqueueSandboxWarmPoolReconcile(ctx, enqueuer, providerID, mode)
		})
		tasks.EnqueueConfiguredWarmPoolReconciles(ctx, enqueuer, deps.Orchestrator)
	}
	ragSched := &ragscheduler.Deps{
		DB:  deps.DB,
		Enq: enqueuer,
		Cfg: ragscheduler.NewConfig(),
	}

	preContextCache := precontext.NewRedisCache(deps.Redis)
	ragDeps := buildRagDeps(ctx, cfg, deps.DB, deps.NangoClient, deps.SpiderClient, deps.KMS, preContextCache)

	workerDeps := &tasks.WorkerDeps{
		DB:           deps.DB,
		Orchestrator: deps.Orchestrator,
		EncKey:       deps.SandboxEncKey,
		EmailSend: func(ctx context.Context, to, subject, body, idempotencyKey string) error {
			return workerSender.Send(ctx, email.Message{
				To:             to,
				Subject:        subject,
				Body:           body,
				IdempotencyKey: idempotencyKey,
			})
		},
		EmailSendTemplate: func(ctx context.Context, to, slug string, variables map[string]string, idempotencyKey string) error {
			return workerSender.SendTemplate(ctx, email.TemplateMessage{
				To:             to,
				Slug:           email.TemplateSlug(slug),
				Variables:      variables,
				IdempotencyKey: idempotencyKey,
			})
		},
		SkillFetcher:    skills.NewGitFetcher(cfg.GitHubToken),
		NangoClient:     deps.NangoClient,
		CacheManager:    deps.CacheManager,
		Credits:         deps.Credits,
		Subscriptions:   deps.Subscriptions,
		Enqueuer:        enqueuer,
		Hindsight:       deps.HindsightClient,
		PreContextCache: preContextCache,
		S3Client:        deps.S3Client,
		EmployeeCompile: employeeruntime.CompileDeps{
			DB:          deps.DB,
			Picker:      credentials.NewPickerWithRegistry(deps.DB, deps.Registry),
			KMS:         deps.KMS,
			EncKey:      deps.SandboxEncKey,
			SigningKey:  deps.SigningKey,
			Cfg:         cfg,
			Hindsight:   deps.HindsightClient,
			Specialists: deps.Specialists,
		},
		Rag:          ragDeps,
		RagScheduler: ragSched,
	}
	if deps.Orchestrator != nil && workerDeps.EmployeeCompile.EncKey != nil {
		deps.Orchestrator.SetEmployeeRuntimeConfigPusher(func(ctx context.Context, sb *model.Sandbox) error {
			return employeeruntime.PushEmployeeRuntimeConfigForSandbox(ctx, workerDeps.EmployeeCompile, sb)
		})
	}
	if deps.Orchestrator != nil && deps.S3Client != nil && workerDeps.EmployeeCompile.EncKey != nil && workerDeps.EmployeeCompile.KMS != nil && cfg.EmployeeSandboxAutoUpgrade {
		if err := tasks.EnqueueEmployeeSandboxAutoUpgrade(ctx, enqueuer, tasks.EmployeeSandboxAutoUpgradePayload{
			RuntimeImage: cfg.SandboxesRuntimeBaseImage,
			Limit:        cfg.EmployeeSandboxAutoUpgradeLimit,
		}); err != nil {
			slog.Error("enqueue employee sandbox auto-upgrade sweep", "error", err)
		}
	}

	mux := tasks.NewServeMux(workerDeps)
	mux.Use(sentryobs.AsynqMiddleware())

	srv := asynq.NewServer(redisOpt, asynq.Config{
		Concurrency: cfg.AsynqConcurrency,
		Queues: map[string]int{
			tasks.QueueCritical:   6,
			tasks.QueueDefault:    3,
			tasks.QueuePeriodic:   2,
			ragtasks.QueueRagWork: 2,
			tasks.QueueBulk:       1,
		},
		Logger:          newAsynqLogger(),
		ShutdownTimeout: cfg.AsynqShutdownTimeout,
		ErrorHandler:    sentryobs.AsynqErrorHandler(),
	})

	errCh := make(chan error, 1)
	goroutine.Go(ctx, func(ctx context.Context) {
		slog.Info("asynq worker starting", "concurrency", cfg.AsynqConcurrency)
		if err := srv.Run(mux); err != nil {
			sentryobs.CaptureAsynqServerError(ctx, err)
			errCh <- err
		}
	})

	var scheduler *asynq.Scheduler
	periodicConfigs := tasks.PeriodicTaskConfigs(cfg, ragSched)
	if len(periodicConfigs) > 0 {
		scheduler = asynq.NewScheduler(redisOpt, sentryobs.AsynqSchedulerOpts(nil))
		for _, pc := range periodicConfigs {
			if _, err := scheduler.Register(pc.Cronspec, pc.Task, pc.Opts...); err != nil {
				return fmt.Errorf("registering periodic task %s: %w", pc.Task.Type(), err)
			}
			slog.Debug("registered periodic task", "type", pc.Task.Type(), "cron", pc.Cronspec)
		}
		if err := scheduler.Start(); err != nil {
			return fmt.Errorf("starting asynq scheduler: %w", err)
		}
		slog.Info("asynq scheduler started", "tasks", len(periodicConfigs))
	}

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"worker"}`))
	})
	healthMux.HandleFunc("/readyz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		sqlDB, err := deps.DB.DB()
		if err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db connection failed"}`))
			return
		}
		if err := sqlDB.Ping(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"db ping failed"}`))
			return
		}
		if err := deps.Redis.Ping(r.Context()).Err(); err != nil {
			w.WriteHeader(http.StatusServiceUnavailable)
			_, _ = w.Write([]byte(`{"status":"error","detail":"redis ping failed"}`))
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"status":"ok","service":"worker"}`))
	})

	// The asynqmon dashboard exposes every queued/archived task payload
	// (customer Slack messages, webhook payloads, emails). It is never mounted on
	// the public health port; it is opt-in, requires HTTP basic-auth credentials,
	// and is served on its own port. ReadOnly only blocks mutations, not reads.
	dashboardSrv := buildAsynqmonServer(ctx, cfg, redisOpt)

	healthPort := cfg.WorkerHealthPort
	if port := os.Getenv("PORT"); port != "" {
		parsed, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("parsing PORT for worker health server: %w", err)
		}
		healthPort = parsed
	}

	healthSrv := &http.Server{
		Addr:              fmt.Sprintf(":%d", healthPort),
		Handler:           healthMux,
		ErrorLog:          sentryobs.NewStdlogBridge("worker_health_server"),
		ReadHeaderTimeout: 5 * time.Second,
	}
	goroutine.Go(ctx, func(context.Context) {
		slog.Info("worker health server starting", "port", healthPort)
		if err := healthSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("worker health server error", "error", err)
		}
	})

	if dashboardSrv != nil {
		goroutine.Go(ctx, func(context.Context) {
			slog.Info("asynqmon dashboard server starting", "addr", dashboardSrv.Addr)
			if err := dashboardSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
				slog.Error("asynqmon dashboard server error", "error", err)
			}
		})
	}

	select {
	case <-ctx.Done():
	case err := <-errCh:
		return err
	}

	slog.Info("worker shutting down")

	// Stop the scheduler first so no new periodic tasks are enqueued during drain.
	if scheduler != nil {
		scheduler.Shutdown()
		slog.Info("asynq scheduler stopped")
	}

	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.AsynqShutdownTimeout)
	defer cancel()

	srv.Shutdown()

	if err := healthSrv.Shutdown(shutdownCtx); err != nil {
		slog.Error("health server shutdown error", "error", err)
	}

	if dashboardSrv != nil {
		if err := dashboardSrv.Shutdown(shutdownCtx); err != nil {
			slog.Error("asynqmon dashboard shutdown error", "error", err)
		}
	}

	slog.Info("worker shutdown complete")
	return nil
}

// buildAsynqmonServer returns an http.Server hosting the asynqmon dashboard
// behind HTTP basic auth on its own port, or nil if the dashboard is disabled
// or misconfigured. The dashboard is fail-closed: it requires both
// HIVY_ASYNQMON_ENABLED=true and basic-auth credentials, and it must not share
// the public health port.
func buildAsynqmonServer(ctx context.Context, cfg *config.Config, redisOpt asynq.RedisConnOpt) *http.Server {
	if !cfg.AsynqmonEnabled {
		slog.Info("asynqmon dashboard disabled (set HIVY_ASYNQMON_ENABLED=true with basic-auth credentials to enable)")
		return nil
	}
	user := strings.TrimSpace(cfg.AsynqmonUser)
	password := cfg.AsynqmonPassword
	if user == "" || password == "" {
		slog.Error("asynqmon dashboard enabled but HIVY_ASYNQMON_USER/HIVY_ASYNQMON_PASSWORD not set — refusing to expose it unauthenticated")
		return nil
	}
	dashboardPort := cfg.AsynqmonPort
	if dashboardPort == cfg.WorkerHealthPort {
		slog.Error("asynqmon dashboard port must differ from the worker health port — refusing to mount on the public health port",
			"asynqmon_port", dashboardPort, "health_port", cfg.WorkerHealthPort)
		return nil
	}
	if port := os.Getenv("PORT"); port != "" {
		if parsed, err := strconv.Atoi(port); err == nil && parsed == dashboardPort {
			slog.Error("asynqmon dashboard port collides with the platform-published PORT — refusing to expose it publicly",
				"asynqmon_port", dashboardPort, "platform_port", parsed)
			return nil
		}
	}

	dashboard := asynqmon.New(asynqmon.Options{
		RootPath:     "/asynq",
		RedisConnOpt: redisOpt,
		ReadOnly:     true,
	})
	mux := http.NewServeMux()
	mux.Handle("/asynq/", basicAuth(user, password, dashboard))
	mux.Handle("/asynq", basicAuth(user, password, dashboard))

	slog.Info("asynqmon dashboard enabled (basic-auth)", "port", dashboardPort, "path", "/asynq")
	return &http.Server{
		Addr:              fmt.Sprintf(":%d", dashboardPort),
		Handler:           mux,
		ErrorLog:          sentryobs.NewStdlogBridge("asynqmon_dashboard"),
		ReadHeaderTimeout: 5 * time.Second,
	}
}

// basicAuth wraps a handler with constant-time HTTP basic-auth verification.
func basicAuth(user, password string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		u, p, ok := r.BasicAuth()
		userOK := subtle.ConstantTimeCompare([]byte(u), []byte(user)) == 1
		passOK := subtle.ConstantTimeCompare([]byte(p), []byte(password)) == 1
		if !ok || !userOK || !passOK {
			w.Header().Set("WWW-Authenticate", `Basic realm="asynqmon"`)
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// asynqLogger adapts slog to asynq's Logger interface.
type asynqLogger struct{}

func newAsynqLogger() *asynqLogger { return &asynqLogger{} }

func (l *asynqLogger) Debug(args ...any) {
	slog.Debug(fmt.Sprint(args...))
}

func (l *asynqLogger) Info(args ...any) {
	slog.Info(fmt.Sprint(args...))
}

func (l *asynqLogger) Warn(args ...any) {
	slog.Warn(fmt.Sprint(args...))
}

func (l *asynqLogger) Error(args ...any) {
	slog.Error(fmt.Sprint(args...))
}

func (l *asynqLogger) Fatal(args ...any) {
	slog.Error(fmt.Sprint(args...))
}
