package config

import (
	"context"
	"os"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
)

func TestAsynqRedisOpt_FailsOnMalformedRedisURL(t *testing.T) {
	cfg := &Config{
		RedisURL: "not-a-valid-redis-url",
	}
	_, err := cfg.AsynqRedisOpt()
	if err == nil {
		t.Fatal("expected error for malformed HIVY_REDIS_URL, got nil")
	}
}

func TestAsynqRedisOpt_ReturnsClientOptWhenURLEmpty(t *testing.T) {
	cfg := &Config{
		RedisAddr:     "localhost:6379",
		RedisPassword: "pw",
		RedisDB:       1,
	}
	opt, err := cfg.AsynqRedisOpt()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opt == nil {
		t.Fatal("expected non-nil RedisConnOpt")
	}
}

func TestAsynqRedisOpt_ParsesValidRedisURL(t *testing.T) {
	cfg := &Config{
		RedisURL: "redis://localhost:6379/0",
	}
	opt, err := cfg.AsynqRedisOpt()
	if err != nil {
		t.Fatalf("unexpected error for valid URL: %v", err)
	}
	if opt == nil {
		t.Fatal("expected non-nil RedisConnOpt")
	}
}

func TestRedisClient_UsesClusterDiscoverySeeds(t *testing.T) {
	cfg := &Config{
		RedisClusterAddrs: []string{"redis-0:6379", "redis-1:6379", "redis-0:6379"},
		RedisPassword:     "pw",
	}
	client, err := cfg.RedisClient()
	if err != nil {
		t.Fatalf("create Redis Cluster client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, ok := client.(*redis.ClusterClient); !ok {
		t.Fatalf("client type = %T, want *redis.ClusterClient", client)
	}

	opt, err := cfg.AsynqRedisOpt()
	if err != nil {
		t.Fatalf("create Asynq Redis Cluster option: %v", err)
	}
	clusterOpt, ok := opt.(asynq.RedisClusterClientOpt)
	if !ok {
		t.Fatalf("Asynq option type = %T, want asynq.RedisClusterClientOpt", opt)
	}
	if len(clusterOpt.Addrs) != 2 {
		t.Fatalf("Asynq cluster seeds = %#v, want deduplicated addresses", clusterOpt.Addrs)
	}
}

func TestRedisClient_UsesStandaloneNodeByDefault(t *testing.T) {
	client, err := (&Config{RedisAddr: "redis:6379"}).RedisClient()
	if err != nil {
		t.Fatalf("create standalone Redis client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })
	if _, ok := client.(*redis.Client); !ok {
		t.Fatalf("client type = %T, want *redis.Client", client)
	}
}

// TestRedisClient_ConnectsToConfiguredCluster is an opt-in integration test.
// It proves that a single seed can discover and connect to every master in a
// live Redis Cluster without making normal test runs depend on Docker.
func TestRedisClient_ConnectsToConfiguredCluster(t *testing.T) {
	seeds := strings.TrimSpace(os.Getenv("HIVY_TEST_REDIS_CLUSTER_ADDRS"))
	if seeds == "" {
		t.Skip("HIVY_TEST_REDIS_CLUSTER_ADDRS is not configured")
	}

	client, err := (&Config{
		RedisCluster:      true,
		RedisClusterAddrs: strings.Split(seeds, ","),
	}).RedisClient()
	if err != nil {
		t.Fatalf("create Redis Cluster client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	cluster, ok := client.(*redis.ClusterClient)
	if !ok {
		t.Fatalf("client type = %T, want *redis.ClusterClient", client)
	}
	var masters atomic.Int64
	if err := cluster.ForEachMaster(t.Context(), func(ctx context.Context, node *redis.Client) error {
		masters.Add(1)
		return node.Ping(ctx).Err()
	}); err != nil {
		t.Fatalf("discover and ping Redis Cluster masters: %v", err)
	}
	masterCount := masters.Load()
	if masterCount < 3 {
		t.Fatalf("discovered %d Redis Cluster masters, want at least 3", masterCount)
	}
	t.Logf("Redis Cluster discovery succeeded: %d masters reachable from %s", masterCount, seeds)
}

func TestAsynqRedisOpt_ConnectsToConfiguredCluster(t *testing.T) {
	seeds := strings.TrimSpace(os.Getenv("HIVY_TEST_REDIS_CLUSTER_ADDRS"))
	if seeds == "" {
		t.Skip("HIVY_TEST_REDIS_CLUSTER_ADDRS is not configured")
	}

	opt, err := (&Config{
		RedisCluster:      true,
		RedisClusterAddrs: strings.Split(seeds, ","),
	}).AsynqRedisOpt()
	if err != nil {
		t.Fatalf("create Asynq Redis Cluster option: %v", err)
	}
	client := asynq.NewClient(opt)
	inspector := asynq.NewInspector(opt)
	queue := "redis-cluster-integration-" + strconv.FormatInt(time.Now().UnixNano(), 10)
	t.Cleanup(func() {
		_ = inspector.DeleteQueue(queue, true)
		_ = inspector.Close()
		_ = client.Close()
	})

	if _, err := client.EnqueueContext(
		t.Context(),
		asynq.NewTask("test:redis-cluster", nil),
		asynq.Queue(queue),
		asynq.MaxRetry(0),
		asynq.Timeout(time.Second),
	); err != nil {
		t.Fatalf("enqueue through Redis Cluster: %v", err)
	}
	info, err := inspector.GetQueueInfo(queue)
	if err != nil {
		t.Fatalf("inspect Redis Cluster queue: %v", err)
	}
	if info.Pending != 1 {
		t.Fatalf("pending tasks = %d, want 1", info.Pending)
	}
}
