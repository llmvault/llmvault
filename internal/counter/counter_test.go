package counter

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func testRedis(t *testing.T) redis.UniversalClient {
	t.Helper()
	rdb := testdb.NewRedisClient()
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		t.Skipf("redis not reachable: %v", err)
	}
	t.Cleanup(func() { rdb.Close() })
	return rdb
}

func testDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("HIVY_DATABASE_URL", "DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Skipf("cannot connect to Postgres: %v", err)
	}
	testdb.ApplyMigrations(t, db)
	return db
}

func uniqueKey(prefix string) string {
	return prefix + ":" + uuid.NewString()
}

// Undo on an expired or never-seeded key must NOT recreate it: a bare INCR would resurrect it as a
// TTL-less counter of 1, capping the token at one request forever.
func TestUndo_ExpiredKeyNotResurrected(t *testing.T) {
	rdb := testRedis(t)
	c := New(rdb, nil)
	ctx := context.Background()

	key := uniqueKey("pbreq:test:expired")

	if err := c.Undo(ctx, key); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	exists, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		t.Fatalf("Exists: %v", err)
	}
	if exists != 0 {
		t.Fatalf("Undo resurrected a missing key (exists=%d); a bare INCR bug would create a TTL-less counter of 1", exists)
	}
}

// Undo must increment a live key and keep its TTL.
func TestUndo_PreservesTTL(t *testing.T) {
	rdb := testRedis(t)
	c := New(rdb, nil)
	ctx := context.Background()

	key := uniqueKey("pbreq:test:live")
	if err := c.Seed(ctx, key, 5, 10*time.Minute); err != nil {
		t.Fatalf("Seed: %v", err)
	}
	if _, err := c.Decrement(ctx, key); err != nil {
		t.Fatalf("Decrement: %v", err)
	}
	if err := c.Undo(ctx, key); err != nil {
		t.Fatalf("Undo: %v", err)
	}

	val, err := c.Read(ctx, key)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if val != 5 {
		t.Fatalf("value = %d, want 5 (decrement then undo)", val)
	}

	ttl, err := rdb.TTL(ctx, key).Result()
	if err != nil {
		t.Fatalf("TTL: %v", err)
	}
	if ttl <= 0 {
		t.Fatalf("TTL = %v, want a positive remaining TTL (Undo must not strip TTL)", ttl)
	}
	t.Cleanup(func() { rdb.Del(ctx, key) })
}

// The loser of the optimistic CAS (RowsAffected==0) must still reconcile Redis
// to the authoritative Postgres value rather than leaving the counter exhausted.
func TestCheckAndRefillToken_ReseedsOnCASLoss(t *testing.T) {
	db := testDB(t)
	rdb := testRedis(t)
	c := New(rdb, db)
	ctx := context.Background()

	tok := seedRefillableToken(t, db)
	past := time.Now().Add(-time.Hour)
	low := int64(0)
	if err := db.Model(&model.Token{}).Where("jti = ?", tok.JTI).
		Updates(map[string]any{"last_refill_at": past, "remaining": low}).Error; err != nil {
		t.Fatalf("prep token: %v", err)
	}

	key := TokKey(tok.JTI)
	if err := c.SeedToken(ctx, tok.JTI, 0, time.Hour); err != nil {
		t.Fatalf("seed redis: %v", err)
	}

	type res struct {
		refilled bool
		err      error
	}
	results := make(chan res, 2)
	for i := 0; i < 2; i++ {
		go func() {
			r, err := c.CheckAndRefillToken(ctx, tok.JTI)
			results <- res{r, err}
		}()
	}
	winners := 0
	for i := 0; i < 2; i++ {
		r := <-results
		if r.err != nil {
			t.Fatalf("CheckAndRefillToken: %v", r.err)
		}
		if r.refilled {
			winners++
		}
	}
	if winners != 1 {
		t.Fatalf("expected exactly 1 CAS winner, got %d", winners)
	}

	val, err := c.Read(ctx, key)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if val != 100 {
		t.Fatalf("redis value = %d, want 100 (CAS-loss path must re-seed from Postgres)", val)
	}
	t.Cleanup(func() { rdb.Del(ctx, key) })
}

func TestCheckAndRefillToken_PerformsRefill(t *testing.T) {
	db := testDB(t)
	rdb := testRedis(t)
	c := New(rdb, db)
	ctx := context.Background()

	tok := seedRefillableToken(t, db)
	past := time.Now().Add(-time.Hour)
	low := int64(0)
	if err := db.Model(&model.Token{}).Where("jti = ?", tok.JTI).
		Updates(map[string]any{"last_refill_at": past, "remaining": low}).Error; err != nil {
		t.Fatalf("prep token: %v", err)
	}

	key := TokKey(tok.JTI)
	if err := c.SeedToken(ctx, tok.JTI, 0, time.Hour); err != nil {
		t.Fatalf("seed redis: %v", err)
	}

	refilled, err := c.CheckAndRefillToken(ctx, tok.JTI)
	if err != nil {
		t.Fatalf("CheckAndRefillToken: %v", err)
	}
	if !refilled {
		t.Fatalf("expected refilled=true")
	}
	val, err := c.Read(ctx, key)
	if err != nil {
		t.Fatalf("Read: %v", err)
	}
	if val != 100 {
		t.Fatalf("redis value = %d, want 100", val)
	}
	t.Cleanup(func() { rdb.Del(ctx, key) })
}

// seedRefillableToken creates an org, credential, and a refill-configured token
// showing a completed refill (last_refill_at = now, remaining = refill amount).
func seedRefillableToken(t *testing.T, db *gorm.DB) model.Token {
	t.Helper()

	org := model.Org{ID: uuid.New(), Name: "counter-test-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	cred := model.Credential{
		ID:           uuid.New(),
		OrgID:        &org.ID,
		Label:        "counter-test",
		BaseURL:      "https://api.openai.com",
		AuthScheme:   "bearer",
		EncryptedKey: []byte("x"),
		WrappedDEK:   []byte("x"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}

	amount := int64(100)
	interval := "1m"
	now := time.Now()
	remaining := int64(100)
	tok := model.Token{
		ID:             uuid.New(),
		OrgID:          org.ID,
		CredentialID:   cred.ID,
		JTI:            uuid.NewString(),
		ExpiresAt:      now.Add(24 * time.Hour),
		Remaining:      &remaining,
		RefillAmount:   &amount,
		RefillInterval: &interval,
		LastRefillAt:   &now,
	}
	if err := db.Create(&tok).Error; err != nil {
		t.Fatalf("create token: %v", err)
	}

	t.Cleanup(func() {
		db.Where("jti = ?", tok.JTI).Delete(&model.Token{})
		db.Where("id = ?", cred.ID).Delete(&model.Credential{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return tok
}
