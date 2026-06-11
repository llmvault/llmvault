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

// The cached limiter must pick up a higher RateLimit without a restart: an org
// throttled at rpm=1, upgraded to a high limit, must stop returning 429.
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

	// burst=1 on the free plan, so the second request is throttled.
	if got := serve(&org); got != http.StatusOK {
		t.Fatalf("first request: got %d, want 200", got)
	}
	if got := serve(&org); got != http.StatusTooManyRequests {
		t.Fatalf("second request (free plan): got %d, want 429", got)
	}

	// Upgrade to 100/sec: SetLimit re-points the cached bucket, so after a short
	// wait the limiter allows again — at the free rate (rps≈0.017) it would not.
	upgraded := org
	upgraded.RateLimit = 6000
	_ = serve(&upgraded) // apply SetLimit/SetBurst to the cached limiter
	time.Sleep(50 * time.Millisecond)
	if got := serve(&upgraded); got != http.StatusOK {
		t.Fatalf("post-upgrade request: got %d, want 200 (SetLimit not applied — token rate still throttled?)", got)
	}
}

// A downgrade is applied in place: an org dropping to rpm=1 starts being throttled.
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

	// Downgrade to rpm=1: SetBurst=1 lowers the ceiling in place.
	low := high
	low.RateLimit = 1
	_ = serve(&low) // re-points the limiter; may still have a token, so drain it
	if got := serve(&low); got != http.StatusTooManyRequests {
		t.Fatalf("post-downgrade request: got %d, want 429 (limit not lowered?)", got)
	}
}
