package config

import (
	"fmt"
	"net/url"

	"github.com/hibiken/asynq"
)

// DatabaseDSN constructs a Postgres connection string from individual fields.
// The password is URL-encoded to handle special characters safely.
func (c *Config) DatabaseDSN() string {
	return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
		url.QueryEscape(c.DBUser),
		url.QueryEscape(c.DBPassword),
		c.DBHost,
		c.DBPort,
		c.DBName,
		c.DBSSLMode,
	)
}

// AsynqRedisOpt returns an asynq.RedisConnOpt from the Redis config fields.
// It returns an error when HIVY_REDIS_URL is set but cannot be parsed, rather
// than silently falling back to a possibly-empty RedisAddr.
func (c *Config) AsynqRedisOpt() (asynq.RedisConnOpt, error) {
	if c.RedisURL != "" {
		opt, err := asynq.ParseRedisURI(c.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("parse HIVY_REDIS_URL for asynq: %w", err)
		}
		return opt, nil
	}
	return asynq.RedisClientOpt{
		Addr:     c.RedisAddr,
		Password: c.RedisPassword,
		DB:       c.RedisDB,
	}, nil
}
