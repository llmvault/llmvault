package db

import (
	"context"
	"fmt"
	"strings"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

func Open(ctx context.Context, dsn string) (*gorm.DB, error) {
	if !IsPostgresDSN(dsn) {
		return nil, fmt.Errorf("HIVY_MICROSANDBOX_DATABASE_DSN is required and must be a Postgres DSN")
	}
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{TranslateError: true})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	if err := sqlDB.PingContext(ctx); err != nil {
		return nil, err
	}
	return db, nil
}

func IsPostgresDSN(dsn string) bool {
	dsn = strings.TrimSpace(dsn)
	return strings.HasPrefix(dsn, "postgres://") ||
		strings.HasPrefix(dsn, "postgresql://") ||
		strings.Contains(dsn, "host=")
}
