package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

// TestGitCredentials_Success verifies that the handler returns properly formatted
// git credentials for GitHub authentication.
func TestGitCredentials_Success(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider": "github-app",
			"credentials": map[string]any{
				"access_token": "ghs_test_installation_token",
				"token_type":   "bearer",
			},
		})
	})

	harness := newGitCredsHarness(t, nangoHandler)

	body := performGitCredsRequest(t, harness)
	if body != "username=x-access-token\npassword=ghs_test_installation_token\n" {
		t.Fatalf("unexpected response body: %q", body)
	}
}

// TestGitCredentials_CachesToken verifies that multiple requests hit the cache
// instead of calling Nango multiple times.
func TestGitCredentials_CachesToken(t *testing.T) {
	callCount := 0
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider": "github-app",
			"credentials": map[string]any{
				"access_token": "ghs_cached_token",
				"expires_at":   time.Now().Add(time.Hour).UTC().Format(time.RFC3339Nano),
			},
		})
	})

	harness := newGitCredsHarness(t, nangoHandler)

	for range 3 {
		body := performGitCredsRequest(t, harness)
		if body != "username=x-access-token\npassword=ghs_cached_token\n" {
			t.Fatalf("unexpected response body: %q", body)
		}
	}

	if callCount != 1 {
		t.Fatalf("expected nango to be called once (cached), got %d calls", callCount)
	}
}

func TestGitCredentials_DoesNotCacheTokenNearExpiry(t *testing.T) {
	callCount := 0
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		callCount++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"provider": "github-app",
			"credentials": map[string]any{
				"access_token": fmt.Sprintf("ghs_near_expiry_token_%d", callCount),
				"expires_at":   time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339Nano),
			},
		})
	})

	harness := newGitCredsHarness(t, nangoHandler)

	firstBody := performGitCredsRequest(t, harness)
	if firstBody != "username=x-access-token\npassword=ghs_near_expiry_token_1\n" {
		t.Fatalf("unexpected first response body: %q", firstBody)
	}
	secondBody := performGitCredsRequest(t, harness)
	if secondBody != "username=x-access-token\npassword=ghs_near_expiry_token_2\n" {
		t.Fatalf("unexpected second response body: %q", secondBody)
	}

	if callCount != 2 {
		t.Fatalf("expected nango to be called twice for near-expiry token, got %d calls", callCount)
	}
}

func TestGitCredentials_DoesNotCacheTokenWithoutUsableExpiry(t *testing.T) {
	tests := []struct {
		name      string
		expiresAt any
		include   bool
	}{
		{name: "missing", include: false},
		{name: "invalid", expiresAt: "not-a-time", include: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			callCount := 0
			nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				callCount++
				creds := map[string]any{
					"access_token": fmt.Sprintf("ghs_uncached_token_%d", callCount),
				}
				if tt.include {
					creds["expires_at"] = tt.expiresAt
				}

				w.Header().Set("Content-Type", "application/json")
				_ = json.NewEncoder(w).Encode(map[string]any{
					"provider":    "github-app",
					"credentials": creds,
				})
			})

			harness := newGitCredsHarness(t, nangoHandler)

			firstBody := performGitCredsRequest(t, harness)
			if firstBody != "username=x-access-token\npassword=ghs_uncached_token_1\n" {
				t.Fatalf("unexpected first response body: %q", firstBody)
			}
			secondBody := performGitCredsRequest(t, harness)
			if secondBody != "username=x-access-token\npassword=ghs_uncached_token_2\n" {
				t.Fatalf("unexpected second response body: %q", secondBody)
			}

			if callCount != 2 {
				t.Fatalf("expected nango to be called twice without usable expiry, got %d calls", callCount)
			}
		})
	}
}
