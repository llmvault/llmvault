package precontext

import (
	"context"
	"fmt"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/redisutil"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestRedisCacheDeletePrefixCoversConfiguredTopology(t *testing.T) {
	ctx := context.Background()
	client := testdb.NewRedisClient()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		t.Skipf("Redis is not available: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	prefix := "precontext:delete-prefix:" + uuid.NewString() + ":"
	keys := make([]string, 0, 32)
	for i := range 32 {
		key := fmt.Sprintf("%s%d", prefix, i)
		keys = append(keys, key)
		if err := client.Set(ctx, key, "value", 0).Err(); err != nil {
			t.Fatalf("seed key %q: %v", key, err)
		}
	}
	keep := "precontext:delete-prefix:keep:" + uuid.NewString()
	if err := client.Set(ctx, keep, "value", 0).Err(); err != nil {
		t.Fatalf("seed unrelated key: %v", err)
	}
	t.Cleanup(func() {
		_ = redisutil.Delete(ctx, client, append(keys, keep)...)
	})

	cache := NewRedisCache(client)
	if err := cache.DeletePrefix(ctx, prefix); err != nil {
		t.Fatalf("delete prefix: %v", err)
	}
	for _, key := range keys {
		exists, err := client.Exists(ctx, key).Result()
		if err != nil {
			t.Fatalf("check key %q: %v", key, err)
		}
		if exists != 0 {
			t.Fatalf("key %q was not deleted", key)
		}
	}
	if exists, err := client.Exists(ctx, keep).Result(); err != nil {
		t.Fatalf("check unrelated key: %v", err)
	} else if exists != 1 {
		t.Fatalf("unrelated key was deleted")
	}
}
