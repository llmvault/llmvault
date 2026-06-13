package main

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/db"
	"github.com/usehivy/hivy/internal/microsandbox/migrations"
)

func runMigrate(ctx context.Context, cfg config.Config, args []string) error {
	if !strings.HasPrefix(cfg.DatabaseDSN, "postgres://") && !strings.Contains(cfg.DatabaseDSN, "host=") {
		return fmt.Errorf("msb migrations require a Postgres HIVY_MICROSANDBOX_DATABASE_DSN")
	}
	subcmd := "up"
	if len(args) > 0 {
		subcmd = args[0]
	}
	gormDB, err := db.Open(ctx, cfg.DatabaseDSN)
	if err != nil {
		return err
	}
	sqlDB, err := gormDB.DB()
	if err != nil {
		return fmt.Errorf("getting sql db: %w", err)
	}
	defer sqlDB.Close()

	switch subcmd {
	case "up":
		results, err := migrations.Up(ctx, sqlDB)
		if err != nil {
			return err
		}
		for _, result := range results {
			slog.Info("msb migration applied", "version", result.Source.Version, "path", result.Source.Path, "duration", result.Duration)
		}
		if len(results) == 0 {
			slog.Info("msb migrations already current")
		}
		return nil
	case "status":
		statuses, err := migrations.Status(ctx, sqlDB)
		if err != nil {
			return err
		}
		for _, status := range statuses {
			slog.Info("msb migration status", "version", status.Source.Version, "path", status.Source.Path, "state", status.State, "applied_at", status.AppliedAt)
		}
		return nil
	case "version":
		version, err := migrations.Version(ctx, sqlDB)
		if err != nil {
			return err
		}
		slog.Info("msb migration version", "version", version)
		return nil
	default:
		return fmt.Errorf("unknown migrate command %q (use: up, status, version)", subcmd)
	}
}
