package handler_test

import (
	"bytes"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/email"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

type refreshTestHarness struct {
	db         *gorm.DB
	router     *chi.Mux
	signingKey []byte
	refreshTTL time.Duration
}

func newRefreshHarness(t *testing.T) *refreshTestHarness {
	t.Helper()

	db := connectTestDB(t)

	pk, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("generate RSA key: %v", err)
	}
	signingKey := []byte("test-signing-key-for-refresh-rotation")
	refreshTTL := 720 * time.Hour

	authHandler := handler.NewAuthHandler(
		db, pk, signingKey,
		"hivy-test", "http://localhost:8080",
		15*time.Minute, refreshTTL,
		&email.NoopSender{},
		"http://localhost:3000",
		true,
		billing.NewCreditsService(db),
	)

	r := chi.NewRouter()
	r.Post("/auth/refresh", authHandler.Refresh)

	return &refreshTestHarness{db: db, router: r, signingKey: signingKey, refreshTTL: refreshTTL}
}

func hashRefresh(token string) string {
	h := sha256.Sum256([]byte(token))
	return hex.EncodeToString(h[:])
}

// seedUserWithRefreshToken creates a user, membership, and a stored refresh token.
func (h *refreshTestHarness) seedUserWithRefreshToken(t *testing.T) (model.User, string) {
	t.Helper()

	user := model.User{
		Email: "refresh-" + uuid.NewString() + "@test.usehivy.com",
		Name:  "Refresh Test",
	}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}

	org := model.Org{
		ID:        uuid.New(),
		Name:      "refresh-org-" + uuid.New().String()[:8],
		RateLimit: 1000,
		Active:    true,
	}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}

	membership := model.OrgMembership{
		OrgID:  org.ID,
		UserID: user.ID,
		Role:   "owner",
	}
	if err := h.db.Create(&membership).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}

	token, err := auth.IssueRefreshToken(h.signingKey, user.ID.String(), h.refreshTTL)
	if err != nil {
		t.Fatalf("issue refresh token: %v", err)
	}
	stored := model.RefreshToken{
		UserID:    user.ID,
		TokenHash: hashRefresh(token),
		ExpiresAt: time.Now().Add(h.refreshTTL),
	}
	if err := h.db.Create(&stored).Error; err != nil {
		t.Fatalf("store refresh token: %v", err)
	}

	t.Cleanup(func() {
		h.db.Where("user_id = ?", user.ID).Delete(&model.RefreshToken{})
		h.db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})

	return user, token
}

func (h *refreshTestHarness) doRefresh(token string) (int, map[string]any) {
	body, _ := json.Marshal(map[string]string{"refresh_token": token})
	req := httptest.NewRequest("POST", "/auth/refresh", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)

	var out map[string]any
	_ = json.Unmarshal(rr.Body.Bytes(), &out)
	return rr.Code, out
}

// Two concurrent presentations of the SAME single-use refresh token must both
// succeed and receive the SAME rotated pair (grace window) — never a 401 that
// would force-log-out the loser of the race.
func TestRefreshConcurrentSameTokenSharesRotatedPair(t *testing.T) {
	h := newRefreshHarness(t)
	_, token := h.seedUserWithRefreshToken(t)

	const n = 8
	var wg sync.WaitGroup
	codes := make([]int, n)
	access := make([]string, n)
	refresh := make([]string, n)

	wg.Add(n)
	for i := 0; i < n; i++ {
		go func(i int) {
			defer wg.Done()
			code, out := h.doRefresh(token)
			codes[i] = code
			if a, ok := out["access_token"].(string); ok {
				access[i] = a
			}
			if r, ok := out["refresh_token"].(string); ok {
				refresh[i] = r
			}
		}(i)
	}
	wg.Wait()

	var wantAccess, wantRefresh string
	for i := 0; i < n; i++ {
		if codes[i] != 200 {
			t.Fatalf("concurrent refresh %d returned %d, want 200 (no force-logout)", i, codes[i])
		}
		if access[i] == "" || refresh[i] == "" {
			t.Fatalf("concurrent refresh %d returned empty tokens", i)
		}
		if wantAccess == "" {
			wantAccess = access[i]
			wantRefresh = refresh[i]
			continue
		}
		// All callers must converge on the same rotated pair or clobber each other.
		if access[i] != wantAccess || refresh[i] != wantRefresh {
			t.Fatalf("concurrent refresh %d got a different rotated pair: pairs diverged", i)
		}
	}

	var newCount int64
	h.db.Model(&model.RefreshToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", hashRefresh(wantRefresh)).
		Count(&newCount)
	if newCount != 1 {
		t.Fatalf("expected exactly 1 live rotated token, got %d", newCount)
	}
}

// The grace window also covers sequential reuse within the window — both calls
// return the same pair.
func TestRefreshSequentialReuseSucceedsWithinGrace(t *testing.T) {
	h := newRefreshHarness(t)
	_, token := h.seedUserWithRefreshToken(t)

	code1, out1 := h.doRefresh(token)
	if code1 != 200 {
		t.Fatalf("first refresh returned %d, want 200", code1)
	}
	code2, out2 := h.doRefresh(token)
	if code2 != 200 {
		t.Fatalf("second refresh (within grace) returned %d, want 200", code2)
	}
	if out1["refresh_token"] != out2["refresh_token"] || out1["access_token"] != out2["access_token"] {
		t.Fatalf("grace reuse returned a different pair")
	}
}

// Once the grace window elapses, replaying the old single-use token is rejected.
func TestRefreshReuseAfterGraceRejected(t *testing.T) {
	h := newRefreshHarness(t)
	_, token := h.seedUserWithRefreshToken(t)

	code1, _ := h.doRefresh(token)
	if code1 != 200 {
		t.Fatalf("first refresh returned %d, want 200", code1)
	}

	stale := time.Now().Add(-1 * time.Hour)
	if err := h.db.Model(&model.RefreshToken{}).
		Where("token_hash = ?", hashRefresh(token)).
		Update("replaced_at", &stale).Error; err != nil {
		t.Fatalf("backdate replaced_at: %v", err)
	}

	code2, out2 := h.doRefresh(token)
	if code2 != 401 {
		t.Fatalf("replay after grace returned %d, want 401", code2)
	}
	if _, ok := out2["access_token"]; ok {
		t.Fatalf("replay after grace must not return tokens")
	}
}

// A token revoked without a recorded replacement (e.g. logout) is rejected.
func TestRefreshRevokedTokenRejected(t *testing.T) {
	h := newRefreshHarness(t)
	_, token := h.seedUserWithRefreshToken(t)

	now := time.Now()
	if err := h.db.Model(&model.RefreshToken{}).
		Where("token_hash = ?", hashRefresh(token)).
		Update("revoked_at", &now).Error; err != nil {
		t.Fatalf("revoke token: %v", err)
	}

	code, out := h.doRefresh(token)
	if code != 401 {
		t.Fatalf("revoked token refresh returned %d, want 401", code)
	}
	if _, ok := out["access_token"]; ok {
		t.Fatalf("revoked token must not return tokens")
	}
}
