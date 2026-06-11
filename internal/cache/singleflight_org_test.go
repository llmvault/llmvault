package cache_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/cache"
)

// When two orgs concurrently resolve the same credential ID on an L1 miss, the
// singleflight must not hand one org the other's secret: a request scoped to a
// different org must fail with not-found, never return orgA's key.
func TestIntegration_CacheManager_SingleflightDoesNotLeakAcrossOrgs(t *testing.T) {
	db := connectTestDB(t)
	rc := connectTestRedis(t)
	kms := createTestKMS(t)
	mgr := buildManager(t, rc, kms, db)

	orgA := createTestOrg(t, db)
	cred := createTestCredential(t, db, kms, orgA.ID, "sk-org-a-secret")
	otherOrg := uuid.New() // not the owning org

	mgr.Memory().Invalidate(cred.ID.String())
	_ = rc.Del(context.Background(), "pbcred:"+cred.ID.String())

	const n = 16
	var wg sync.WaitGroup
	var leaked atomic.Int32
	var leakMu sync.Mutex

	for i := 0; i < n; i++ {
		wg.Add(1)
		owner := i%2 == 0
		go func(owner bool) {
			defer wg.Done()
			org := otherOrg
			if owner {
				org = orgA.ID
			}
			res, err := mgr.GetDecryptedCredential(context.Background(), cred.ID.String(), org)
			if owner {
				if err != nil || string(res.APIKey) != "sk-org-a-secret" {
					leakMu.Lock()
					t.Errorf("owner request failed: err=%v key=%q", err, safeKey(res))
					leakMu.Unlock()
				}
				return
			}
			// Non-owner: must NOT receive orgA's secret.
			if err == nil && res != nil && string(res.APIKey) == "sk-org-a-secret" {
				leaked.Add(1)
			}
		}(owner)
	}
	wg.Wait()

	if leaked.Load() != 0 {
		t.Fatalf("cross-org credential leak: %d non-owner requests received orgA's secret", leaked.Load())
	}
}

func safeKey(c *cache.DecryptedCredential) string {
	if c == nil {
		return ""
	}
	return string(c.APIKey)
}
