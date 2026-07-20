package db

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

type PoolConfig struct {
	MaxOpenConnections int
	MaxIdleConnections int
	ConnectionLifetime time.Duration
}

func Open(ctx context.Context, dsn string, poolConfigs ...PoolConfig) (*gorm.DB, error) {
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
	if len(poolConfigs) > 0 {
		pool := poolConfigs[0]
		if pool.MaxOpenConnections > 0 {
			sqlDB.SetMaxOpenConns(pool.MaxOpenConnections)
		}
		if pool.MaxIdleConnections > 0 {
			sqlDB.SetMaxIdleConns(pool.MaxIdleConnections)
		}
		if pool.ConnectionLifetime > 0 {
			sqlDB.SetConnMaxLifetime(pool.ConnectionLifetime)
		}
	}
	return db, nil
}

func IsPostgresDSN(dsn string) bool {
	dsn = strings.TrimSpace(dsn)
	return strings.HasPrefix(dsn, "postgres://") ||
		strings.HasPrefix(dsn, "postgresql://") ||
		strings.Contains(dsn, "host=")
}
