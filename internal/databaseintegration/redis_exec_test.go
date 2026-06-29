package databaseintegration

import (
	"context"
	"errors"
	"testing"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/testdb"
)

func TestExecuteRedisReadsThroughProxyCommand(t *testing.T) {
	ctx := context.Background()
	addr := testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")
	client := redis.NewClient(&redis.Options{Addr: addr})
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

func TestExecuteRedisReturnsNilForMissingKey(t *testing.T) {
	ctx := context.Background()
	addr := testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")
	client := redis.NewClient(&redis.Options{Addr: addr})
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
