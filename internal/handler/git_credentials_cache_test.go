package handler_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

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
