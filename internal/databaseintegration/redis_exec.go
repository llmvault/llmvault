package databaseintegration

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/redis/go-redis/v9"
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
	client, err := openRedis(dsn)
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
	client, err := openRedis(dsn)
	if err != nil {
		return err
	}
	defer client.Close()
	return client.Ping(ctx).Err()
}

func IntrospectRedis(ctx context.Context, dsn string) ([]RedisKeyInfo, error) {
	client, err := openRedis(dsn)
	if err != nil {
		return nil, err
	}
	defer client.Close()
	iter := client.Scan(ctx, 0, "", redisIntrospectionSampleLimit).Iterator()
	out := make([]RedisKeyInfo, 0, redisIntrospectionSampleLimit)
	for iter.Next(ctx) {
		key := iter.Val()
		keyType, err := client.Type(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("inspect Redis key %q type: %w", key, err)
		}
		info := RedisKeyInfo{Key: key, Type: keyType}
		ttl, err := client.TTL(ctx, key).Result()
		if err != nil {
			return nil, fmt.Errorf("inspect Redis key %q ttl: %w", key, err)
		}
		if ttl > 0 {
			seconds := int64(ttl.Seconds())
			info.TTLSeconds = &seconds
		}
		out = append(out, info)
		if len(out) >= redisIntrospectionSampleLimit {
			break
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("scan Redis keys: %w", err)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Key < out[j].Key })
	return out, nil
}

func openRedis(dsn string) (*redis.Client, error) {
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
	return redis.NewClient(opts), nil
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
