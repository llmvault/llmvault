package config

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

// DatabaseDSN constructs a Postgres connection string, URL-encoding the password.
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
	if c.RedisClusterEnabled() {
		opts, err := c.redisClusterOptions()
		if err != nil {
			return nil, err
		}
		return asynq.RedisClusterClientOpt{
			Addrs:        opts.Addrs,
			MaxRedirects: opts.MaxRedirects,
			Username:     opts.Username,
			Password:     opts.Password,
			DialTimeout:  opts.DialTimeout,
			ReadTimeout:  opts.ReadTimeout,
			WriteTimeout: opts.WriteTimeout,
			TLSConfig:    opts.TLSConfig,
		}, nil
	}
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

// RedisClient creates a Redis client for either a standalone node or a Redis
// Cluster. Cluster clients discover the complete topology from their seed
// nodes, so callers only need to provide reachable initial addresses.
func (c *Config) RedisClient() (redis.UniversalClient, error) {
	if c.RedisClusterEnabled() {
		opts, err := c.redisClusterOptions()
		if err != nil {
			return nil, err
		}
		applyRedisClusterPoolDefaults(opts)
		return redis.NewClusterClient(opts), nil
	}

	var opts *redis.Options
	var err error
	if c.RedisURL != "" {
		opts, err = redis.ParseURL(c.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("parse HIVY_REDIS_URL: %w", err)
		}
	} else {
		opts = &redis.Options{Addr: c.RedisAddr, Password: c.RedisPassword, DB: c.RedisDB}
	}
	applyRedisPoolDefaults(opts)
	return redis.NewClient(opts), nil
}

func (c *Config) redisClusterOptions() (*redis.ClusterOptions, error) {
	var opts *redis.ClusterOptions
	var err error
	if c.RedisURL != "" {
		opts, err = redis.ParseClusterURL(c.RedisURL)
		if err != nil {
			return nil, fmt.Errorf("parse HIVY_REDIS_URL for Redis Cluster: %w", err)
		}
	} else {
		opts = &redis.ClusterOptions{Password: c.RedisPassword}
	}

	if c.RedisAddr != "" {
		opts.Addrs = append(opts.Addrs, c.RedisAddr)
	}
	opts.Addrs = append(opts.Addrs, c.RedisClusterAddrs...)
	opts.Addrs = uniqueRedisAddrs(opts.Addrs)
	if len(opts.Addrs) == 0 {
		return nil, fmt.Errorf("at least one Redis Cluster seed address is required")
	}
	if opts.Password == "" {
		opts.Password = c.RedisPassword
	}
	return opts, nil
}

func applyRedisPoolDefaults(opts *redis.Options) {
	if opts.PoolSize == 0 {
		opts.PoolSize = 500
	}
	if opts.MinIdleConns == 0 {
		opts.MinIdleConns = 20
	}
	if opts.PoolTimeout == 0 {
		opts.PoolTimeout = 4 * time.Second
	}
}

func applyRedisClusterPoolDefaults(opts *redis.ClusterOptions) {
	if opts.PoolSize == 0 {
		opts.PoolSize = 500
	}
	if opts.MinIdleConns == 0 {
		opts.MinIdleConns = 20
	}
	if opts.PoolTimeout == 0 {
		opts.PoolTimeout = 4 * time.Second
	}
}

func uniqueRedisAddrs(addrs []string) []string {
	result := make([]string, 0, len(addrs))
	for _, addr := range addrs {
		addr = strings.TrimSpace(addr)
		if addr != "" && !slices.Contains(result, addr) {
			result = append(result, addr)
		}
	}
	return result
}
