package main

import (
	"context"
	"fmt"
	"os"

	"github.com/caarlos0/env/v11"

	"github.com/usehivy/hivy/internal/db"
	"github.com/usehivy/hivy/internal/devseed"
	"github.com/usehivy/hivy/internal/logging"
)

type config struct {
	Enabled         bool   `env:"HIVY_DEV_SEED_ENABLED" envDefault:"false"`
	Environment     string `env:"HIVY_ENVIRONMENT" envDefault:"development"`
	DatabaseURL     string `env:"HIVY_DATABASE_URL,required"`
	APIURL          string `env:"HIVY_DEV_SEED_API_URL" envDefault:"http://api:8080"`
	Email           string `env:"HIVY_DEV_SEED_EMAIL" envDefault:"dev@hivy.local"`
	Password        string `env:"HIVY_DEV_SEED_PASSWORD" envDefault:"local-development"`
	UserName        string `env:"HIVY_DEV_SEED_USER_NAME" envDefault:"Local Developer"`
	OrgName         string `env:"HIVY_DEV_SEED_ORG_NAME" envDefault:"Hivy Development"`
	PromptCompany   string `env:"HIVY_DEV_SEED_PROMPT_COMPANY" envDefault:"A local Hivy workspace for product development and testing."`
	TeamName        string `env:"HIVY_DEV_SEED_TEAM_NAME" envDefault:"Development"`
	TeamDescription string `env:"HIVY_DEV_SEED_TEAM_DESCRIPTION" envDefault:"The default local development team."`
}

func main() {
	ctx := context.Background()
	if err := run(ctx); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "development seed failed", "error", err)
		os.Exit(1)
	}
}

func run(ctx context.Context) error {
	var cfg config
	if err := env.Parse(&cfg); err != nil {
		return fmt.Errorf("parse development seed configuration: %w", err)
	}
	database, err := db.New(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	sqlDB, err := database.DB()
	if err != nil {
		return fmt.Errorf("get development seed database: %w", err)
	}
	defer sqlDB.Close()

	result, err := devseed.Reconcile(ctx, devseed.Config{
		Enabled: cfg.Enabled, Environment: cfg.Environment, APIURL: cfg.APIURL,
		Email: cfg.Email, Password: cfg.Password, UserName: cfg.UserName,
		OrgName: cfg.OrgName, PromptCompany: cfg.PromptCompany,
		TeamName: cfg.TeamName, TeamDescription: cfg.TeamDescription,
	}, devseed.NewGORMStore(database))
	if err != nil {
		return err
	}
	//nolint:forbidigo // user-facing output consumed by the local start script
	fmt.Printf("development seed ready email=%s org_id=%s team_id=%s user_created=%t team_created=%t\n",
		result.Email, result.OrgID, result.TeamID, result.UserCreated, result.TeamCreated)
	return nil
}
