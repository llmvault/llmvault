package main

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/bootstrap"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
	"github.com/usehivy/hivy/internal/precontext"
	"github.com/usehivy/hivy/internal/sandbox"
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
	agentCompile := agentruntime.CompileDeps{
		DB:         deps.DB,
		Picker:     credentials.NewPickerWithRegistry(deps.DB, deps.Registry),
		KMS:        deps.KMS,
		EncKey:     deps.SandboxEncKey,
		SigningKey: deps.SigningKey,
		Cfg:        cfg,
		Hindsight:  deps.HindsightClient,
	}
	var orgAgentSyncer tasks.OrgHivyAgentSyncer
	var agentHandler *handler.AgentHandler
	if deps.Orchestrator != nil && agentCompile.EncKey != nil {
		agentHandler = handler.NewAgentHandler(deps.DB, deps.Orchestrator, agentCompile, deps.Registry)
		agentHandler.SetEnqueuer(enqueuer)
		if deps.HindsightClient != nil {
			agentHandler.SetMemoryProvisioner(hindsight.NewBankProvisioner(deps.DB, deps.HindsightClient))
		}
		orgAgentSyncer = agentHandler
	}

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
		OrgAgentSyncer:  orgAgentSyncer,
		S3Client:        deps.S3Client,
		AgentCompile:    agentCompile,
		Rag:             ragDeps,
		RagScheduler:    ragSched,
	}
	if deps.Orchestrator != nil && workerDeps.AgentCompile.EncKey != nil {
		deps.Orchestrator.SetAgentRuntimeConfigPusher(func(ctx context.Context, sb *model.Sandbox) error {
			return agentruntime.PushAgentRuntimeConfigForSandbox(ctx, workerDeps.AgentCompile, sb)
		})
	}
	if deps.Orchestrator != nil && deps.S3Client != nil && workerDeps.AgentCompile.EncKey != nil && workerDeps.AgentCompile.KMS != nil && cfg.AgentSandboxAutoUpgrade {
		if err := tasks.EnqueueAgentSandboxAutoUpgrade(ctx, enqueuer, tasks.AgentSandboxAutoUpgradePayload{
			RuntimeImage: sandbox.AgentRuntimeTemplateRef(cfg),
			Limit:        cfg.AgentSandboxAutoUpgradeLimit,
		}); err != nil {
			slog.Error("enqueue agent sandbox auto-upgrade sweep", "error", err)
		}
	}

	mux := tasks.NewServeMux(workerDeps)
	if agentHandler != nil {
		mux.HandleFunc(tasks.TypePluginInstallSync,
			handler.NewPluginInstallSyncHandler(deps.DB, agentHandler, enqueuer, deps.Orchestrator, agentCompile).Handle)
	}
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

	// The asynqmon dashboard exposes every queued/archived task payload (customer
	// messages, webhooks, emails), so it is opt-in, basic-auth protected, and on its
	// own port. ReadOnly only blocks mutations, not reads.
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
