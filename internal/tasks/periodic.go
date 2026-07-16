package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/rag/scheduler"
	"github.com/usehivy/hivy/internal/sandbox"
)

// PeriodicTaskConfigs returns the recurring tasks for the asynq Scheduler. Every
// config carries asynq.Unique(interval) so N worker-replica schedulers don't
// enqueue N duplicate ticks within a tick window.
func PeriodicTaskConfigs(cfg *config.Config, ragSched *scheduler.Deps) []*asynq.PeriodicTaskConfig {
	configs := []*asynq.PeriodicTaskConfig{
		{
			Cronspec: "0 */6 * * *", // every 6 hours
			Task:     asynq.NewTask(TypeTokenCleanup, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(2),
				asynq.Timeout(2 * time.Minute),
				asynq.Unique(6 * time.Hour),
			},
		},
		{
			Cronspec: "@every 1m",
			Task:     asynq.NewTask(TypeSessionReflectionScan, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(time.Minute),
				asynq.Unique(time.Minute),
			},
		},
		{
			Cronspec: "@every 30s",
			Task:     asynq.NewTask(TypeBillingBatchProcess, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(5 * time.Minute),
				asynq.Unique(30 * time.Second),
			},
		},
		{
			// Stranded-facts sweep: finds channels with unconsolidated reflection
			// facts (consolidated_at IS NULL) and re-enqueues consolidation. The
			// primary trigger is the immediate post-reflection enqueue; this
			// catches anything that slipped through.
			Cronspec: "@every 5m",
			Task:     asynq.NewTask(TypeMemoryConsolidationSweep, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(2 * time.Minute),
				asynq.Unique(5 * time.Minute),
			},
		},
		{
			// Nightly observation expiry: archives observations whose expires_at
			// has passed and refreshes the affected channel memory digests.
			Cronspec: "0 3 * * *",
			Task:     asynq.NewTask(TypeMemoryObservationExpire, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(2),
				asynq.Timeout(10 * time.Minute),
				asynq.Unique(time.Hour),
			},
		},
		{
			// Zero-usage reconcile: backfills token counts on OpenRouter system
			// generations that returned no usage (intermittent under account-level
			// BYOK) from the authoritative /api/v1/generation endpoint so the billing
			// batch can charge them.
			Cronspec: "@every 2m",
			Task:     asynq.NewTask(TypeGenerationReconcile, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(5 * time.Minute),
				asynq.Unique(2 * time.Minute),
			},
		},
	}

	if sandboxPeriodicTasksConfigured(cfg) {
		interval := cfg.SandboxResourceCheckInterval
		if interval > 0 {
			configs = append(configs, &asynq.PeriodicTaskConfig{
				Cronspec: fmt.Sprintf("@every %s", interval),
				Task:     asynq.NewTask(TypeSandboxResourceCheck, nil),
				Opts: []asynq.Option{
					asynq.Queue(QueuePeriodic),
					asynq.MaxRetry(1),
					asynq.Timeout(5 * time.Minute),
					asynq.Unique(interval),
				},
			})
		}

		// Sandbox reaper: releases leaked paid compute the inline cleanup missed
		// (stuck creating/error and stranded warm slots).
		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: "@every 5m",
			Task:     asynq.NewTask(TypeSandboxReap, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(10 * time.Minute),
				asynq.Unique(5 * time.Minute),
			},
		})

		// Auto-sleep: stops sandboxes whose sessions have been idle with no events
		// for the idle threshold; they wake transparently on the next request.
		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: "@every 30s",
			Task:     asynq.NewTask(TypeSandboxAutoSleep, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(2 * time.Minute),
				// Keep the uniqueness lease longer than the task timeout so a slow
				// runner stop cannot allow a second sweep to overlap the first.
				asynq.Unique(3 * time.Minute),
			},
		})

		// Reconcile: re-syncs our sandbox status mirror with the control plane so
		// gateway-driven wakes (which never notify the Go API) don't strand
		// sandboxes outside the auto-sleep sweep.
		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: "@every 2m30s",
			Task:     asynq.NewTask(TypeSandboxReconcile, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(2 * time.Minute),
				asynq.Unique(150 * time.Second),
			},
		})

		// Turn watchdog: resets sessions stuck in an 'active' turn (lost terminal
		// event) so a leaked turn stops exempting its sandbox from auto-sleep.
		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: "@every 1m",
			Task:     asynq.NewTask(TypeSessionTurnWatchdog, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(time.Minute),
				asynq.Unique(time.Minute),
			},
		})
	}

	if sandboxPeriodicTasksConfigured(cfg) {
		scheduleScanInterval := cfg.AgentScheduleScanInterval
		if scheduleScanInterval <= 0 {
			scheduleScanInterval = 5 * time.Second
		}
		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: fmt.Sprintf("@every %s", scheduleScanInterval),
			Task:     asynq.NewTask(TypeAgentScheduleScan, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(time.Minute),
				asynq.Unique(scheduleScanInterval),
			},
		})
	}

	if ragSched != nil {
		configs = append(configs, ragSched.Configs()...)
	}
	return configs
}

func sandboxPeriodicTasksConfigured(cfg *config.Config) bool {
	if cfg == nil || strings.TrimSpace(cfg.SandboxEncryptionKey) == "" {
		return false
	}
	switch strings.TrimSpace(cfg.SandboxProviderID) {
	case sandbox.ProviderDocker:
		return strings.TrimSpace(cfg.SandboxDockerRuntimeOrigin) != ""
	case sandbox.ProviderDaytona:
		return strings.TrimSpace(cfg.DaytonaAPIKey) != ""
	case sandbox.ProviderRailway:
		return strings.TrimSpace(cfg.RailwayAPIToken) != "" &&
			strings.TrimSpace(cfg.RailwayProjectID) != "" &&
			strings.TrimSpace(cfg.RailwayEnvironmentID) != "" &&
			strings.TrimSpace(sandbox.AgentRuntimeImageRef(cfg, model.SandboxImageDefault)) != ""
	case sandbox.ProviderMicrosandbox:
		return strings.TrimSpace(cfg.MicrosandboxControlURL) != "" &&
			strings.TrimSpace(cfg.MicrosandboxControlAPIToken) != ""
	default:
		return false
	}
}
