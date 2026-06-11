package handler_test

import (
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// A narrow API key must not be able to mint a key with the broad "all" scope.
func TestAPIKeyHandler_Create_NarrowKeyCannotEscalateToAll(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequestWithAPIKey(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "escalate",
		"scopes": []string{"all"},
	}, &org, []string{"credentials"})

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403 escalation block, got %d; body: %s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	_ = json.NewDecoder(rr.Body).Decode(&body)
	if !strings.Contains(body["error"], "broader scopes") {
		t.Fatalf("expected broader-scopes error, got %q", body["error"])
	}
}

// A narrow API key must not mint scopes it does not itself hold.
func TestAPIKeyHandler_Create_NarrowKeyCannotMintForeignScope(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequestWithAPIKey(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "foreign-scope",
		"scopes": []string{"credentials", "tokens"},
	}, &org, []string{"credentials"})

	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// An API key may mint a key with scopes it already holds (equal or narrower).
func TestAPIKeyHandler_Create_KeyMayMintEqualOrNarrowerScopes(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequestWithAPIKey(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "narrower",
		"scopes": []string{"credentials"},
	}, &org, []string{"credentials", "tokens"})

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
}

// An "all"-scoped API key may mint any scope.
func TestAPIKeyHandler_Create_AllScopeKeyMayMintAnything(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequestWithAPIKey(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "from-all",
		"scopes": []string{"all"},
	}, &org, []string{"all"})

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d; body: %s", rr.Code, rr.Body.String())
	}
}
