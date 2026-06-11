package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// TestRateLimit_PlanChangeRaisesCeiling verifies the cached limiter picks up a
// new (higher) RateLimit without a process restart (P2-36). An org throttled
// at rpm=1 is upgraded to a high limit and must stop returning 429.
func TestRateLimit_PlanChangeRaisesCeiling(t *testing.T) {
	orgID := uuid.New()
	org := model.Org{ID: orgID, Name: "rl-plan-change", RateLimit: 1, Active: true}

	rl := middleware.RateLimit()
	handler := rl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	serve := func(o *model.Org) int {
		req := middleware.WithOrg(httptest.NewRequest(http.MethodGet, "/", nil), o)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	// burst = max(1/10,1) = 1, so the second request on the free plan is throttled.
	if got := serve(&org); got != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", got)
	}
	if got := serve(&org); got != http.StatusTooManyRequests {
		t.Fatalf("second request (free plan): got %d, want 429", got)
	}

	// Plan upgrade: same org ID, much higher rpm (6000/min = 100/sec). The first
	// request after the upgrade triggers SetLimit, re-pointing the existing
	// bucket at the higher rate (it may be throttled because the bucket is still
	// empty from the free-plan window). Tokens then accrue at 100/sec, so after a
	// short wait the limiter allows again — proof the new ceiling is live without
	// a process restart. On the free plan this same wait leaves the bucket empty
	// (rps≈0.017), so this asserts the rate actually changed.
	upgraded := org
	upgraded.RateLimit = 6000
	_ = serve(&upgraded) // apply SetLimit/SetBurst to the cached limiter
	time.Sleep(50 * time.Millisecond)
	if got := serve(&upgraded); got != http.StatusOK {
		t.Fatalf("post-upgrade request: got %d, want 200 (SetLimit not applied — token rate still throttled?)", got)
	}
}

// TestRateLimit_PlanChangeLowersCeiling verifies a downgrade is also applied in
// place: an org on a high plan that drops to rpm=1 starts being throttled.
func TestRateLimit_PlanChangeLowersCeiling(t *testing.T) {
	orgID := uuid.New()
	high := model.Org{ID: orgID, Name: "rl-downgrade", RateLimit: 6000, Active: true}

	rl := middleware.RateLimit()
	handler := rl(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	serve := func(o *model.Org) int {
		req := middleware.WithOrg(httptest.NewRequest(http.MethodGet, "/", nil), o)
		rr := httptest.NewRecorder()
		handler.ServeHTTP(rr, req)
		return rr.Code
	}

	if got := serve(&high); got != http.StatusOK {
		t.Fatalf("high-plan request: got %d, want 200", got)
	}

	// Downgrade to rpm=1 (burst=1). The token consumed above plus the new low
	// burst means the next two requests after the downgrade are throttled.
	low := high
	low.RateLimit = 1
	// First request after downgrade re-points the limiter (SetBurst=1) and may
	// still have a token; drain it.
	_ = serve(&low)
	if got := serve(&low); got != http.StatusTooManyRequests {
		t.Fatalf("post-downgrade request: got %d, want 429 (limit not lowered?)", got)
	}
}
