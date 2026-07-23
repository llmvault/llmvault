package databaseintegration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/redisutil"
)

const redisIntrospectionSampleLimit = 100

type RedisResult struct {
	Command   string `json:"command"`
	Result    any    `json:"result"`
	RowCount  int    `json:"row_count,omitempty"`
	Truncated bool   `json:"truncated,omitempty"`
}

type RedisKeyInfo struct {
	Key        string `json:"key"`
	Type       string `json:"type"`
	TTLSeconds *int64 `json:"ttl_seconds,omitempty"`
}

func ExecuteRedis(ctx context.Context, dsn string, body []byte, policy Policy) (RedisResult, error) {
	cmd, err := ValidateRedisCommand(body, policy)
	if err != nil {
		return RedisResult{}, err
	}
	client, err := openRedis(ctx, dsn)
	if err != nil {
		return RedisResult{}, err
	}
	defer client.Close()
	args := make([]any, 0, len(cmd.Args)+1)
	args = append(args, cmd.Command)
	for _, arg := range cmd.Args {
		args = append(args, arg)
	}
	value, err := client.Do(ctx, args...).Result()
	if err != nil && !errors.Is(err, redis.Nil) {
		return RedisResult{}, fmt.Errorf("execute Redis command: %w", err)
	}
	if errors.Is(err, redis.Nil) {
		value = nil
	}
	normalized, count, truncated := normalizeRedisValue(value, redisLimit(policy.MaxRows))
	return RedisResult{Command: cmd.Command, Result: normalized, RowCount: count, Truncated: truncated}, nil
}

func TestRedis(ctx context.Context, dsn string) error {
	client, err := openRedis(ctx, dsn)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Ping(ctx).Err()
}

func IntrospectRedis(ctx context.Context, dsn string) ([]RedisKeyInfo, error) {
	client, err := openRedis(ctx, dsn)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	out := make([]RedisKeyInfo, 0, redisIntrospectionSampleLimit)
	var mu sync.Mutex
	err = redisutil.Scan(ctx, client, "", redisIntrospectionSampleLimit, func(ctx context.Context, _ redis.UniversalClient, keys []string) error {
		for _, key := range keys {
			mu.Lock()
			if len(out) >= redisIntrospectionSampleLimit {
				mu.Unlock()
				return nil
			}
			mu.Unlock()
			keyType, err := client.Type(ctx, key).Result()
			if err != nil {
				return fmt.Errorf("inspect Redis key %q type: %w", key, err)
			}
			info := RedisKeyInfo{Key: key, Type: keyType}
			ttl, err := client.TTL(ctx, key).Result()
			if err != nil {
				return fmt.Errorf("inspect Redis key %q ttl: %w", key, err)
			}
			if ttl > 0 {
				seconds := int64(ttl.Seconds())
				info.TTLSeconds = &seconds
			}
			mu.Lock()
			if len(out) < redisIntrospectionSampleLimit {
				out = append(out, info)
			}
			mu.Unlock()
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("scan Redis keys: %w", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func openRedis(ctx context.Context, dsn string) (redis.UniversalClient, error) {
	opts, err := redis.ParseURL(dsn)
	if err != nil {
		if strings.Contains(dsn, "://") {
			return nil, fmt.Errorf("parse Redis URL: %w", err)
		}
		opts = &redis.Options{Addr: strings.TrimSpace(dsn)}
	}
	if strings.TrimSpace(opts.Addr) == "" {
		return nil, fmt.Errorf("Redis address is required")
	}
	client := redis.NewClient(opts)
	if _, err := client.ClusterInfo(ctx).Result(); err != nil {
		if isClusterDisabledError(err) {
			return client, nil
		}
		_ = client.Close()
		return nil, fmt.Errorf("detect Redis topology: %w", err)
	}
	_ = client.Close()
	if opts.DB != 0 {
		return nil, fmt.Errorf("Redis Cluster only supports database 0")
	}
	return redis.NewClusterClient(&redis.ClusterOptions{
		Addrs:                      []string{opts.Addr},
		ClientName:                 opts.ClientName,
		Dialer:                     opts.Dialer,
		OnConnect:                  opts.OnConnect,
		Protocol:                   opts.Protocol,
		Username:                   opts.Username,
		Password:                   opts.Password,
		CredentialsProvider:        opts.CredentialsProvider,
		CredentialsProviderContext: opts.CredentialsProviderContext,
		MaxRetries:                 opts.MaxRetries,
		MinRetryBackoff:            opts.MinRetryBackoff,
		MaxRetryBackoff:            opts.MaxRetryBackoff,
		DialTimeout:                opts.DialTimeout,
		ReadTimeout:                opts.ReadTimeout,
		WriteTimeout:               opts.WriteTimeout,
		PoolFIFO:                   opts.PoolFIFO,
		PoolSize:                   opts.PoolSize,
		PoolTimeout:                opts.PoolTimeout,
		MinIdleConns:               opts.MinIdleConns,
		MaxIdleConns:               opts.MaxIdleConns,
		MaxActiveConns:             opts.MaxActiveConns,
		ConnMaxIdleTime:            opts.ConnMaxIdleTime,
		ConnMaxLifetime:            opts.ConnMaxLifetime,
		ReadBufferSize:             opts.ReadBufferSize,
		WriteBufferSize:            opts.WriteBufferSize,
		TLSConfig:                  opts.TLSConfig,
	}), nil
}

func isClusterDisabledError(err error) bool {
	message := strings.ToLower(err.Error())
	return strings.Contains(message, "cluster support disabled") ||
		strings.Contains(message, "unknown command")
}

func normalizeRedisValue(value any, limit int) (any, int, bool) {
	switch typed := value.(type) {
	case []any:
		out := make([]any, 0, min(len(typed), limit))
		truncated := len(typed) > limit
		for idx, item := range typed {
			if idx >= limit {
				break
			}
			normalized, _, childTruncated := normalizeRedisValue(item, limit)
			truncated = truncated || childTruncated
			out = append(out, normalized)
		}
		return out, len(typed), truncated
	case []string:
		if len(typed) > limit {
			return typed[:limit], len(typed), true
		}
		return typed, len(typed), false
	case []byte:
		return string(typed), 1, false
	default:
		return typed, 0, false
	}
}
