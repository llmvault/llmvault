package config

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/hibiken/asynq"
	"github.com/redis/go-redis/v9"
	"github.com/usehivy/hivy/internal/testdb"
)

func setRequiredEnv(t *testing.T) {
	t.Helper()
	t.Setenv("HIVY_PORT", "8080")
	t.Setenv("HIVY_LOG_LEVEL", "info")
	t.Setenv("HIVY_LOG_FORMAT", "json")
	t.Setenv("HIVY_DB_HOST", "localhost")
	t.Setenv("HIVY_DB_USER", "user")
	t.Setenv("HIVY_DB_PASSWORD", "pass")
	t.Setenv("HIVY_DB_NAME", "db")
	t.Setenv("HIVY_KMS_TYPE", "aead")
	t.Setenv("HIVY_KMS_KEY", "dGVzdC1rZXktMzItYnl0ZXMtbG9uZy1lbm91Z2gh")
	t.Setenv("HIVY_REDIS_ADDR", testdb.DefaultRedisAddr)
	t.Setenv("HIVY_REDIS_DB", "0")
	t.Setenv("HIVY_REDIS_CACHE_TTL", "30m")
	// HIVY_REDIS_URL is not set here — HIVY_REDIS_ADDR is used as fallback for local dev
	t.Setenv("HIVY_MEM_CACHE_TTL", "5m")
	t.Setenv("HIVY_MEM_CACHE_MAX_SIZE", "10000")
	t.Setenv("HIVY_JWT_SIGNING_KEY", "test-signing-key")
	t.Setenv("HIVY_CORS_ORIGINS", "http://localhost:3000")
	t.Setenv("HIVY_AUTH_RSA_PRIVATE_KEY", "dGVzdC1wZW0=")
	t.Setenv("HIVY_FRONTEND_URL", "http://localhost:3000")
	t.Setenv("HIVY_NANGO_WEBHOOKS_SECRET", "test-nango-webhook-secret")
	t.Setenv("HIVY_PREVIEW_BASE_DOMAIN", "preview.usehivy.test")
}

// The Nango webhook signing secret must be required so the server fails to boot
// rather than running with an empty secret, which would let attackers forge
// webhook signatures (HMAC computed with an empty key). See verifyNangoSignature.
func TestLoad_RequiresNangoWebhooksSecret(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HIVY_NANGO_WEBHOOKS_SECRET", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected error when HIVY_NANGO_WEBHOOKS_SECRET is unset")
	}
}

// Load must error when neither HIVY_REDIS_URL nor HIVY_REDIS_ADDR is set.
func TestLoad_NoRedisConfig(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HIVY_REDIS_ADDR", "")
	t.Setenv("HIVY_REDIS_URL", "")

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when neither HIVY_REDIS_URL nor HIVY_REDIS_ADDR is set")
	}
}

func TestLoad_AcceptsRedisClusterSeedsWithoutStandaloneAddress(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HIVY_REDIS_ADDR", "")
	t.Setenv("HIVY_REDIS_URL", "")
	t.Setenv("HIVY_REDIS_CLUSTER_ADDRS", "redis-0:6379,redis-1:6379")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !cfg.RedisClusterEnabled() {
		t.Fatal("Redis Cluster should be enabled when seed addresses are configured")
	}
}

func TestLoad_RejectsRedisDatabaseInClusterMode(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HIVY_REDIS_CLUSTER", "true")
	t.Setenv("HIVY_REDIS_DB", "1")

	if _, err := Load(); err == nil {
		t.Fatal("expected non-zero Redis database to be rejected in Redis Cluster mode")
	}
}

func TestLoad_AddsFrontendURLToCORSOrigins(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HIVY_CORS_ORIGINS", "https://admin.usehivy.test")
	t.Setenv("HIVY_FRONTEND_URL", "https://usehivy.test")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if !containsString(cfg.CORSOrigins, "https://admin.usehivy.test") {
		t.Fatalf("configured CORS origin missing: %#v", cfg.CORSOrigins)
	}
	if !containsString(cfg.CORSOrigins, "https://usehivy.test") {
		t.Fatalf("frontend URL was not added to CORS origins: %#v", cfg.CORSOrigins)
	}
}

func TestLoad_DoesNotDuplicateFrontendCORSOrigin(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HIVY_CORS_ORIGINS", "https://usehivy.test")
	t.Setenv("HIVY_FRONTEND_URL", "https://usehivy.test/")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if len(cfg.CORSOrigins) != 1 || cfg.CORSOrigins[0] != "https://usehivy.test" {
		t.Fatalf("CORS origins = %#v", cfg.CORSOrigins)
	}
}

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
	masters := 0
	if err := cluster.ForEachMaster(t.Context(), func(ctx context.Context, node *redis.Client) error {
		masters++
		return node.Ping(ctx).Err()
	}); err != nil {
		t.Fatalf("discover and ping Redis Cluster masters: %v", err)
	}
	if masters < 3 {
		t.Fatalf("discovered %d Redis Cluster masters, want at least 3", masters)
	}
	t.Logf("Redis Cluster discovery succeeded: %d masters reachable from %s", masters, seeds)
}

func TestLoad_WarnOnDisableSSLModeInProduction(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HIVY_ENVIRONMENT", "production")
	t.Setenv("HIVY_DB_SSLMODE", "disable")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DBSSLMode != "disable" {
		t.Fatalf("DBSSLMode = %q, want disable", cfg.DBSSLMode)
	}
}

func TestLoad_NoWarnOnRequireSSLModeInProduction(t *testing.T) {
	setRequiredEnv(t)
	t.Setenv("HIVY_ENVIRONMENT", "production")
	t.Setenv("HIVY_DB_SSLMODE", "require")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load config: %v", err)
	}
	if cfg.DBSSLMode != "require" {
		t.Fatalf("DBSSLMode = %q, want require", cfg.DBSSLMode)
	}
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
