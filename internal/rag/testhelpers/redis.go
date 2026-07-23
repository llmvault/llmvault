package testhelpers

import (
	"context"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/redisutil"
	"github.com/usehivy/hivy/internal/testdb"
)

// testRedisDB steers standalone test fixtures away from production's default
// DB 0. Redis Cluster only supports DB 0.
const testRedisDB = 11

func RedisAddr() string {
	return testdb.RedisAddr("HIVY_REDIS_ADDR", "TEST_REDIS_ADDR")
}

func RedisDB() int {
	if testdb.RedisClusterEnabled() {
		return 0
	}
	return testRedisDB
}

func ConnectTestRedis(t *testing.T) redis.UniversalClient {
	t.Helper()

	cli := testdb.NewRedisClient()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := cli.Ping(ctx).Err(); err != nil {
		t.Fatalf("Redis not reachable (run `make test-services-up`): %v", err)
	}
	if err := redisutil.FlushDB(ctx, cli); err != nil {
		t.Fatalf("flush test redis DB %d: %v", RedisDB(), err)
	}
	t.Cleanup(func() {
		flushCtx, flushCancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer flushCancel()
		_ = redisutil.FlushDB(flushCtx, cli)
		_ = cli.Close()
	})
	return cli
}
