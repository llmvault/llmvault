package redisutil

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// IsCluster reports whether client routes commands through Redis Cluster.
func IsCluster(client redis.UniversalClient) bool {
	_, ok := client.(*redis.ClusterClient)
	return ok
}

// Delete removes keys on either Redis topology. Redis Cluster rejects a
// multi-key DEL when the keys span hash slots, so cluster deletes are emitted
// as one-key commands through a topology-aware pipeline.
func Delete(ctx context.Context, client redis.UniversalClient, keys ...string) error {
	if client == nil || len(keys) == 0 {
		return nil
	}
	if !IsCluster(client) || len(keys) == 1 {
		return client.Del(ctx, keys...).Err()
	}
	_, err := client.Pipelined(ctx, func(pipe redis.Pipeliner) error {
		for _, key := range keys {
			pipe.Del(ctx, key)
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("delete Redis Cluster keys: %w", err)
	}
	return nil
}

// Scan calls fn for every matching key. A standalone SCAN covers the complete
// keyspace; Redis Cluster requires scanning every master independently.
func Scan(
	ctx context.Context,
	client redis.UniversalClient,
	pattern string,
	count int64,
	fn func(context.Context, redis.UniversalClient, []string) error,
) error {
	if client == nil || fn == nil {
		return nil
	}
	if cluster, ok := client.(*redis.ClusterClient); ok {
		if err := cluster.ForEachMaster(ctx, func(ctx context.Context, master *redis.Client) error {
			return scanNode(ctx, master, pattern, count, fn)
		}); err != nil {
			return fmt.Errorf("scan Redis Cluster masters: %w", err)
		}
		return nil
	}
	return scanNode(ctx, client, pattern, count, fn)
}

func scanNode(
	ctx context.Context,
	client redis.UniversalClient,
	pattern string,
	count int64,
	fn func(context.Context, redis.UniversalClient, []string) error,
) error {
	var cursor uint64
	for {
		keys, next, err := client.Scan(ctx, cursor, pattern, count).Result()
		if err != nil {
			return err
		}
		if len(keys) > 0 {
			if err := fn(ctx, client, keys); err != nil {
				return err
			}
		}
		if next == 0 {
			return nil
		}
		cursor = next
	}
}

// FlushDB clears the selected standalone database or every Redis Cluster
// master. It is intended for isolated integration-test databases only.
func FlushDB(ctx context.Context, client redis.UniversalClient) error {
	if client == nil {
		return nil
	}
	if cluster, ok := client.(*redis.ClusterClient); ok {
		if err := cluster.ForEachMaster(ctx, func(ctx context.Context, master *redis.Client) error {
			return master.FlushDB(ctx).Err()
		}); err != nil {
			return fmt.Errorf("flush Redis Cluster masters: %w", err)
		}
		return nil
	}
	return client.FlushDB(ctx).Err()
}
