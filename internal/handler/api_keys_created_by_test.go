package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// TestAPIKeyCreate_RecordsCreatedBy verifies that a JWT-authenticated creator is
// attributed on the api_keys.created_by column.
func TestAPIKeyCreate_RecordsCreatedBy(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)
	creator := createTestUser(t, h.db, "apikey-creator-"+uuid.NewString()[:8]+"@example.com")

	body := map[string]any{"name": "attributed-key", "scopes": []string{"connect"}}
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/api-keys", &buf)
	req.Header.Set("Content-Type", "application/json")
	req = middleware.WithOrg(req, &org)
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: creator.ID.String(), OrgID: org.ID.String()})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: got %d, body %s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&created)

	var key model.APIKey
	if err := h.db.Where("id = ?", created.ID).First(&key).Error; err != nil {
		t.Fatalf("load key: %v", err)
	}
	if key.CreatedBy == nil {
		t.Fatal("created_by must be set for a JWT-authenticated creator")
	}
	if *key.CreatedBy != creator.ID {
		t.Fatalf("created_by = %s, want %s", *key.CreatedBy, creator.ID)
	}
}

// TestAPIKeyCreate_APIKeyCallerHasNullCreatedBy verifies that a key minted by an
// API-key-authenticated caller (no human actor) records a NULL created_by.
func TestAPIKeyCreate_APIKeyCallerHasNullCreatedBy(t *testing.T) {
	h := newAPIKeyHarness(t)
	org := createTestOrg(t, h.db)

	rr := h.doRequestWithAPIKey(t, http.MethodPost, "/v1/api-keys", map[string]any{
		"name":   "machine-key",
		"scopes": []string{"connect"},
	}, &org, []string{"all"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: got %d, body %s", rr.Code, rr.Body.String())
	}
	var created struct {
		ID string `json:"id"`
	}
	_ = json.NewDecoder(rr.Body).Decode(&created)

	var key model.APIKey
	if err := h.db.Where("id = ?", created.ID).First(&key).Error; err != nil {
		t.Fatalf("load key: %v", err)
	}
	if key.CreatedBy != nil {
		t.Fatalf("created_by must be NULL for an API-key caller, got %s", *key.CreatedBy)
	}
}
