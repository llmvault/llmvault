package databaseintegration

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/redisutil"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestExecuteRedisReadsThroughProxyCommand(t *testing.T) {
	ctx := context.Background()
	addr := testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")
	client := testdb.NewRedisClient()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	key := "databaseintegration:test:user:1"
	if err := client.Set(ctx, key, "Ada", 0).Err(); err != nil {
		t.Fatalf("seed redis key: %v", err)
	}
	t.Cleanup(func() { _ = client.Del(ctx, key).Err() })

	result, err := ExecuteRedis(ctx, addr, []byte(`{"command":"GET","args":["`+key+`"]}`), Policy{
		AllowedKeys: []string{"databaseintegration:test:*"},
		MaxRows:     10,
	})
	if err != nil {
		t.Fatalf("ExecuteRedis returned error: %v", err)
	}
	if result.Command != "GET" || result.Result != "Ada" {
		t.Fatalf("result = %#v", result)
	}
}

func TestIntrospectRedisScansConfiguredTopology(t *testing.T) {
	ctx := context.Background()
	addr := testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")
	client := testdb.NewRedisClient()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	prefix := "databaseintegration:introspect:" + uuid.NewString() + ":"
	keys := make([]string, 0, 32)
	for i := range 32 {
		key := fmt.Sprintf("%s%d", prefix, i)
		keys = append(keys, key)
		if err := client.Set(ctx, key, "value", 0).Err(); err != nil {
			t.Fatalf("seed Redis key %q: %v", key, err)
		}
	}
	t.Cleanup(func() { _ = redisutil.Delete(ctx, client, keys...) })

	info, err := IntrospectRedis(ctx, addr)
	if err != nil {
		t.Fatalf("introspect Redis: %v", err)
	}
	found := make([]string, 0, len(keys))
	for _, entry := range info {
		if slices.Contains(keys, entry.Key) {
			found = append(found, entry.Key)
		}
	}
	if len(found) != len(keys) {
		t.Fatalf("introspection found %d/%d topology-spanning keys", len(found), len(keys))
	}
}

func TestExecuteRedisReturnsNilForMissingKey(t *testing.T) {
	ctx := context.Background()
	addr := testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")
	client := testdb.NewRedisClient()
	if err := client.Ping(ctx).Err(); err != nil {
		t.Skipf("redis not reachable at %s: %v", addr, err)
	}
	t.Cleanup(func() { _ = client.Close() })

	result, err := ExecuteRedis(ctx, addr, []byte(`{"command":"GET","args":["databaseintegration:test:missing"]}`), Policy{
		AllowedKeys: []string{"databaseintegration:test:*"},
	})
	if err != nil && !errors.Is(err, redis.Nil) {
		t.Fatalf("ExecuteRedis returned error: %v", err)
	}
	if result.Result != nil {
		t.Fatalf("missing key result = %#v, want nil", result.Result)
	}
}
