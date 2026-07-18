package precontext

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

type RedisCache struct {
	Client redis.UniversalClient
}

func NewRedisCache(client redis.UniversalClient) *RedisCache {
	if client == nil {
		return nil
	}
	return &RedisCache{Client: client}
}

func (c *RedisCache) Get(ctx context.Context, key string) (string, bool, error) {
	if c == nil || c.Client == nil {
		return "", false, nil
	}
	value, err := c.Client.Get(ctx, key).Result()
	if err == nil {
		return value, true, nil
	}
	if err == redis.Nil {
		return "", false, nil
	}
	return "", false, err
}

func (c *RedisCache) Set(ctx context.Context, key, value string, ttl time.Duration) error {
	if c == nil || c.Client == nil {
		return nil
	}
	return c.Client.Set(ctx, key, value, ttl).Err()
}

func (c *RedisCache) Del(ctx context.Context, keys ...string) error {
	if c == nil || c.Client == nil || len(keys) == 0 {
		return nil
	}
	return c.Client.Del(ctx, keys...).Err()
}

func (c *RedisCache) DeletePrefix(ctx context.Context, prefix string) error {
	if c == nil || c.Client == nil || prefix == "" {
		return nil
	}
	var cursor uint64
	for {
		keys, next, err := c.Client.Scan(ctx, cursor, prefix+"*", 100).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := c.Client.Del(ctx, keys...).Err(); err != nil {
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

func SessionsCacheKey(orgID, agentID uuid.UUID) string {
	return fmt.Sprintf("agent_precontext:sessions:%s:%s", orgID, agentID)
}

func KnowledgeCacheKey(orgID, agentID uuid.UUID, query string) string {
	return fmt.Sprintf("agent_precontext:knowledge:%s:%s:%s", orgID, agentID, queryHash(query))
}

func InvalidateSessions(ctx context.Context, cache Cache, orgID, agentID uuid.UUID) {
	_ = cacheDel(ctx, cache, SessionsCacheKey(orgID, agentID))
}

func InvalidateKnowledge(ctx context.Context, cache Cache, orgID uuid.UUID) {
	if cache == nil || orgID == uuid.Nil {
		return
	}
	_ = cache.DeletePrefix(ctx, fmt.Sprintf("agent_precontext:knowledge:%s:", orgID))
}

func cacheDel(ctx context.Context, cache Cache, key string) error {
	if cache == nil {
		return nil
	}
	return cache.Del(ctx, key)
}
