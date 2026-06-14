package tasks

import (
	"fmt"
	"strings"
	"time"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/config"
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
			Cronspec: "@every 15m",
			Task:     asynq.NewTask(TypeCreditsExpire, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(2),
				asynq.Timeout(10 * time.Minute),
				asynq.Unique(15 * time.Minute),
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
			// Subscription renewal sweep: enqueues per-sub tasks that own the attempt
			// counter. Filtering on last_renewal_attempt_at caps attempts per interval.
			Cronspec: "@every 1h",
			Task:     asynq.NewTask(TypeBillingRenewSweep, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(5 * time.Minute),
				asynq.Unique(time.Hour),
			},
		},
	}

	if sandboxPeriodicTasksConfigured(cfg) {
		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: "@every 30s",
			Task:     asynq.NewTask(TypeSandboxHealthCheck, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(time.Minute),
				asynq.Unique(30 * time.Second),
			},
		})

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

		// Sandbox lifecycle policy (every 5 min): stops sandboxes idle >10 min,
		// archives sandboxes stopped >24 h.
		configs = append(configs, &asynq.PeriodicTaskConfig{
			Cronspec: "@every 5m",
			Task:     asynq.NewTask(TypeSandboxLifecycle, nil),
			Opts: []asynq.Option{
				asynq.Queue(QueuePeriodic),
				asynq.MaxRetry(1),
				asynq.Timeout(10 * time.Minute),
				asynq.Unique(5 * time.Minute),
			},
		})

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
			strings.TrimSpace(cfg.SandboxesRuntimeBaseImage) != ""
	default:
		return false
	}
}
