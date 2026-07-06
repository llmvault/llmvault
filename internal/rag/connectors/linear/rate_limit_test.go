package linear

import (
	"context"
	"net/http"
	"strconv"
	"testing"
	"time"
)

func TestRateLimitedThenRetries(t *testing.T) {
	fp := newFakeProxy()
	// First hit: HTTP 400 with a RATELIMITED GraphQL error. Second hit
	// (after wait) succeeds.
	fp.stub("TeamIssues", 400, `{"errors":[{"message":"rate limited","extensions":{"code":"RATELIMITED"}}]}`)
	fp.stub("TeamIssues", 200, `{"data":{"issues":{"nodes":[{"id":"i1"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	issues, _, err := c.TeamIssues(context.Background(), []string{"t1"}, nil, "")
	if err != nil {
		t.Fatalf("TeamIssues after throttle: %v", err)
	}
	if len(issues) != 1 || issues[0].ID != "i1" {
		t.Fatalf("unexpected issues: %+v", issues)
	}
	if len(sleeps) != 1 {
		t.Fatalf("expected exactly 1 rate-limit wait, got %v", sleeps)
	}
	// No reset header => default wait.
	if sleeps[0] != defaultRateLimitWait {
		t.Errorf("expected default wait %s, got %s", defaultRateLimitWait, sleeps[0])
	}
	if got := len(fp.requestBodies()); got != 2 {
		t.Errorf("expected 2 requests, got %d", got)
	}
}

func TestRateLimitedExhaustsAttempts(t *testing.T) {
	fp := newFakeProxy()
	// Always throttled — the trailing stub repeats.
	fp.stub("Teams", 400, `{"errors":[{"message":"nope","extensions":{"code":"RATELIMITED"}}]}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	if _, err := c.Teams(context.Background()); err == nil {
		t.Fatal("expected error after exhausting rate-limit attempts")
	}
	if got := len(fp.requestBodies()); got != maxRateLimitAttempts {
		t.Errorf("expected %d attempts, got %d", maxRateLimitAttempts, got)
	}
}

func TestTransient5xxRetries(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("Teams", 500, `internal error`)
	fp.stub("Teams", 200, `{"data":{"teams":{"nodes":[{"id":"t1","name":"One","key":"ONE"}],"pageInfo":{"hasNextPage":false,"endCursor":""}}}}`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	teams, err := c.Teams(context.Background())
	if err != nil {
		t.Fatalf("Teams after 5xx: %v", err)
	}
	if len(teams) != 1 {
		t.Fatalf("expected 1 team, got %d", len(teams))
	}
	if len(sleeps) != 1 {
		t.Errorf("expected 1 backoff sleep, got %v", sleeps)
	}
}

func TestTransient5xxExhausts(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("Teams", 503, `unavailable`)

	var sleeps []time.Duration
	c := newTestClient(fp, &sleeps)
	if _, err := c.Teams(context.Background()); err == nil {
		t.Fatal("expected error after exhausting transient attempts")
	}
	if got := len(fp.requestBodies()); got != maxTransientAttempts {
		t.Errorf("expected %d attempts, got %d", maxTransientAttempts, got)
	}
}

func TestDetectRateLimit(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	// 200 OK is never a throttle.
	if _, ok := detectRateLimit(200, http.Header{}, []byte(`{"data":{}}`), now); ok {
		t.Error("200 should not be a throttle")
	}
	// 400 without RATELIMITED code is not a throttle.
	if _, ok := detectRateLimit(400, http.Header{}, []byte(`{"errors":[{"message":"bad","extensions":{"code":"OTHER"}}]}`), now); ok {
		t.Error("400 without RATELIMITED should not throttle")
	}
	// 400 RATELIMITED is a throttle; no header => default wait.
	if wait, ok := detectRateLimit(400, http.Header{}, []byte(`{"errors":[{"extensions":{"code":"RATELIMITED"}}]}`), now); !ok || wait != defaultRateLimitWait {
		t.Errorf("400 RATELIMITED: ok=%v wait=%s", ok, wait)
	}
	// 429 is a defensive throttle.
	if _, ok := detectRateLimit(429, http.Header{}, nil, now); !ok {
		t.Error("429 should throttle")
	}
}

func TestRateLimitWaitMagnitude(t *testing.T) {
	now := time.Date(2026, 7, 6, 12, 0, 0, 0, time.UTC)

	// Unix seconds, 20s in the future.
	sec := strconv.FormatInt(now.Add(20*time.Second).Unix(), 10)
	if w := rateLimitWait(header("X-RateLimit-Requests-Reset", sec), now); w != 20*time.Second {
		t.Errorf("seconds reset: want 20s, got %s", w)
	}
	// Unix milliseconds, 15s in the future.
	ms := strconv.FormatInt(now.Add(15*time.Second).UnixMilli(), 10)
	if w := rateLimitWait(header("X-RateLimit-Requests-Reset", ms), now); w != 15*time.Second {
		t.Errorf("millis reset: want 15s, got %s", w)
	}
	// Far-future reset is capped.
	far := strconv.FormatInt(now.Add(10*time.Minute).Unix(), 10)
	if w := rateLimitWait(header("X-RateLimit-Requests-Reset", far), now); w != maxRateLimitWait {
		t.Errorf("capped reset: want %s, got %s", maxRateLimitWait, w)
	}
	// Past reset => 0 (retry immediately).
	past := strconv.FormatInt(now.Add(-time.Minute).Unix(), 10)
	if w := rateLimitWait(header("X-RateLimit-Requests-Reset", past), now); w != 0 {
		t.Errorf("past reset: want 0, got %s", w)
	}
	// Missing header => default.
	if w := rateLimitWait(http.Header{}, now); w != defaultRateLimitWait {
		t.Errorf("missing header: want %s, got %s", defaultRateLimitWait, w)
	}
}

func header(k, v string) http.Header {
	h := http.Header{}
	h.Set(k, v)
	return h
}
