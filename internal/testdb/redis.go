package testdb

import (
	"os"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

const DefaultRedisClusterAddrs = "redis-1.localhost:16279,redis-2.localhost:16280,redis-3.localhost:16281"

// NewRedisClient builds the same Redis topology selected for the test process.
// CI may continue to provide standalone Redis, while docker-compose-backed
// tests use the production-shaped three-master cluster.
func NewRedisClient() redis.UniversalClient {
	if RedisClusterEnabled() {
		return redis.NewClusterClient(&redis.ClusterOptions{
			Addrs:    RedisClusterAddrs(),
			Password: RedisPassword(),
		})
	}
	return redis.NewClient(&redis.Options{
		Addr:     RedisAddr(),
		Password: RedisPassword(),
		DB:       redisDB(),
	})
}

func RedisPassword() string {
	return strings.TrimSpace(os.Getenv("HIVY_REDIS_PASSWORD"))
}

func RedisClusterEnabled() bool {
	if raw := strings.TrimSpace(os.Getenv("HIVY_REDIS_CLUSTER")); raw != "" {
		enabled, err := strconv.ParseBool(raw)
		return err == nil && enabled
	}
	return strings.TrimSpace(os.Getenv("HIVY_REDIS_CLUSTER_ADDRS")) != ""
}

func RedisClusterAddrs() []string {
	raw := strings.TrimSpace(os.Getenv("HIVY_REDIS_CLUSTER_ADDRS"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("HIVY_TEST_REDIS_CLUSTER_ADDRS"))
	}
	if raw == "" {
		raw = DefaultRedisClusterAddrs
	}
	parts := strings.Split(raw, ",")
	addrs := make([]string, 0, len(parts))
	for _, part := range parts {
		if addr := strings.TrimSpace(part); addr != "" {
			addrs = append(addrs, addr)
		}
	}
	return addrs
}

func redisDB() int {
	raw := strings.TrimSpace(os.Getenv("HIVY_REDIS_DB"))
	if raw == "" {
		return 0
	}
	db, err := strconv.Atoi(raw)
	if err != nil {
		return 0
	}
	return db
}
