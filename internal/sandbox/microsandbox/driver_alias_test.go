package microsandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/sandbox"
)

func TestDriverClaimAndReleaseAlias(t *testing.T) {
	var claimReq map[string]any
	var deleteCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Authorization"); got != "Bearer test-token" {
			t.Fatalf("Authorization = %q", got)
		}
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/v1/aliases/my-app":
			if err := json.NewDecoder(r.Body).Decode(&claimReq); err != nil {
				t.Fatalf("decode claim: %v", err)
			}
			_ = json.NewEncoder(w).Encode(map[string]any{
				"alias":      "my-app",
				"url":        "https://my-app.preview.test",
				"sandbox_id": "sbx_test",
				"port":       8080,
			})
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/aliases/my-app":
			deleteCalled = true
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	driver, err := NewDriver(Config{ControlURL: server.URL, APIToken: "test-token", RuntimeImage: "img"})
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}

	// AliasProvider is satisfied.
	var _ sandbox.AliasProvider = driver

	url, err := driver.ClaimAlias(context.Background(), "my-app", "sbx_test", 8080)
	if err != nil {
		t.Fatalf("ClaimAlias: %v", err)
	}
	if url != "https://my-app.preview.test" {
		t.Fatalf("ClaimAlias url = %q", url)
	}
	if claimReq["sandbox_id"] != "sbx_test" || claimReq["port"] != float64(8080) {
		t.Fatalf("claim request = %#v", claimReq)
	}

	if err := driver.ReleaseAlias(context.Background(), "my-app"); err != nil {
		t.Fatalf("ReleaseAlias: %v", err)
	}
	if !deleteCalled {
		t.Fatal("ReleaseAlias did not call DELETE")
	}
}

// TestDriverClaimAliasSurfacesNon2xx proves the apps driver treats a control
// non-2xx (the fail-closed response when the gateway push fails) as an error,
// so the deploy is marked failed rather than reporting a dead alias URL.
func TestDriverClaimAliasSurfacesNon2xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":"failed to propagate alias route to gateway"}`, http.StatusBadGateway)
	}))
	defer server.Close()

	driver, err := NewDriver(Config{ControlURL: server.URL, APIToken: "test-token", RuntimeImage: "img"})
	if err != nil {
		t.Fatalf("NewDriver: %v", err)
	}

	url, err := driver.ClaimAlias(context.Background(), "my-app", "sbx_test", 8080)
	if err == nil {
		t.Fatalf("ClaimAlias returned nil error on 502; url = %q", url)
	}
	if url != "" {
		t.Fatalf("ClaimAlias url = %q, want empty on error", url)
	}
}
