package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func mountOrgEnvTestRoutes(r chi.Router, orgHandler *handler.OrgHandler) {
	r.Get("/current/environment-variables", orgHandler.ListEnvironmentVariables)
	r.Post("/current/environment-variables", orgHandler.CreateEnvironmentVariable)
	r.Patch("/current/environment-variables/{name}", orgHandler.UpdateEnvironmentVariable)
	r.Delete("/current/environment-variables/{name}", orgHandler.DeleteEnvironmentVariable)
}

func (h *orgUpdateHarness) doEnvRequest(t *testing.T, method, path string, userID, orgID uuid.UUID, role string, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	if body != nil {
		_ = json.NewEncoder(buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", orgID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: userID.String(),
		OrgID:  orgID.String(),
		Role:   role,
	})

	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestOrgEnvironmentVariables_CRUDUsesEncryptedAgentEnv(t *testing.T) {
	h := newOrgUpdateHarness(t)
	encKey := newTestEncKey(t)
	h.orgHandler.SetEnvironmentEncryptionKey(encKey)
	org, user := h.createOrg(t, "admin")

	createRR := h.doEnvRequest(t, http.MethodPost, "/v1/orgs/current/environment-variables", user.ID, org.ID, "admin", map[string]any{
		"name":  "stripe_api_key",
		"value": "sk_test_123",
	})
	if createRR.Code != http.StatusCreated {
		t.Fatalf("create status: got %d body=%s, want 201", createRR.Code, createRR.Body.String())
	}
	var created struct {
		Name   string `json:"name"`
		EnvKey string `json:"env_key"`
	}
	if err := json.Unmarshal(createRR.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create: %v", err)
	}
	if created.Name != "STRIPE_API_KEY" || created.EnvKey != "HIVY_ORG_STRIPE_API_KEY" {
		t.Fatalf("created = %+v, want STRIPE_API_KEY/HIVY_ORG_STRIPE_API_KEY", created)
	}
	envVars := decryptOrgTestEnvVars(t, h, encKey, org.ID)
	if envVars["HIVY_ORG_STRIPE_API_KEY"] != "sk_test_123" {
		t.Fatalf("encrypted env HIVY_ORG_STRIPE_API_KEY = %q, want sk_test_123", envVars["HIVY_ORG_STRIPE_API_KEY"])
	}
	envVars["AGENT_INTERNAL_ONLY"] = "keep-me"
	saveOrgTestEnvVars(t, h, encKey, org.ID, envVars)

	listRR := h.doEnvRequest(t, http.MethodGet, "/v1/orgs/current/environment-variables", user.ID, org.ID, "admin", nil)
	if listRR.Code != http.StatusOK {
		t.Fatalf("list status: got %d body=%s, want 200", listRR.Code, listRR.Body.String())
	}
	var listed struct {
		Data []struct {
			Name   string `json:"name"`
			EnvKey string `json:"env_key"`
		} `json:"data"`
	}
	if err := json.Unmarshal(listRR.Body.Bytes(), &listed); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(listed.Data) != 1 || listed.Data[0].Name != "STRIPE_API_KEY" {
		t.Fatalf("listed = %+v, want only STRIPE_API_KEY custom env", listed.Data)
	}

	updateRR := h.doEnvRequest(t, http.MethodPatch, "/v1/orgs/current/environment-variables/STRIPE_API_KEY", user.ID, org.ID, "admin", map[string]any{
		"name":  "billing_secret",
		"value": "bill_secret_456",
	})
	if updateRR.Code != http.StatusOK {
		t.Fatalf("update status: got %d body=%s, want 200", updateRR.Code, updateRR.Body.String())
	}
	envVars = decryptOrgTestEnvVars(t, h, encKey, org.ID)
	if _, exists := envVars["HIVY_ORG_STRIPE_API_KEY"]; exists {
		t.Fatal("old env key should be removed after rename")
	}
	if envVars["HIVY_ORG_BILLING_SECRET"] != "bill_secret_456" {
		t.Fatalf("renamed env = %q, want bill_secret_456", envVars["HIVY_ORG_BILLING_SECRET"])
	}
	if envVars["AGENT_INTERNAL_ONLY"] != "keep-me" {
		t.Fatalf("non-custom env = %q, want keep-me", envVars["AGENT_INTERNAL_ONLY"])
	}

	deleteRR := h.doEnvRequest(t, http.MethodDelete, "/v1/orgs/current/environment-variables/BILLING_SECRET", user.ID, org.ID, "admin", nil)
	if deleteRR.Code != http.StatusOK {
		t.Fatalf("delete status: got %d body=%s, want 200", deleteRR.Code, deleteRR.Body.String())
	}
	envVars = decryptOrgTestEnvVars(t, h, encKey, org.ID)
	if _, exists := envVars["HIVY_ORG_BILLING_SECRET"]; exists {
		t.Fatal("custom env should be removed after delete")
	}
	if envVars["AGENT_INTERNAL_ONLY"] != "keep-me" {
		t.Fatalf("non-custom env after delete = %q, want keep-me", envVars["AGENT_INTERNAL_ONLY"])
	}
}

func TestOrgEnvironmentVariables_RejectsPrefixedAndInvalidNames(t *testing.T) {
	h := newOrgUpdateHarness(t)
	h.orgHandler.SetEnvironmentEncryptionKey(newTestEncKey(t))
	org, user := h.createOrg(t, "admin")

	prefixedRR := h.doEnvRequest(t, http.MethodPost, "/v1/orgs/current/environment-variables", user.ID, org.ID, "admin", map[string]any{
		"name":  "HIVY_ORG_TOKEN",
		"value": "secret",
	})
	if prefixedRR.Code != http.StatusBadRequest {
		t.Fatalf("prefixed status: got %d body=%s, want 400", prefixedRR.Code, prefixedRR.Body.String())
	}
	if !strings.Contains(prefixedRR.Body.String(), "must not include") {
		t.Fatalf("prefixed body = %s, want prefix validation message", prefixedRR.Body.String())
	}

	invalidRR := h.doEnvRequest(t, http.MethodPost, "/v1/orgs/current/environment-variables", user.ID, org.ID, "admin", map[string]any{
		"name":  "1BAD-NAME",
		"value": "secret",
	})
	if invalidRR.Code != http.StatusBadRequest {
		t.Fatalf("invalid status: got %d body=%s, want 400", invalidRR.Code, invalidRR.Body.String())
	}
}

func TestOrgEnvironmentVariables_RequiresOrgAdmin(t *testing.T) {
	h := newOrgUpdateHarness(t)
	h.orgHandler.SetEnvironmentEncryptionKey(newTestEncKey(t))
	org, user := h.createOrg(t, "member")

	rr := h.doEnvRequest(t, http.MethodGet, "/v1/orgs/current/environment-variables", user.ID, org.ID, "member", nil)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status: got %d body=%s, want 403", rr.Code, rr.Body.String())
	}
}

func decryptOrgTestEnvVars(t *testing.T, h *orgUpdateHarness, encKey interface {
	DecryptString([]byte) (string, error)
}, orgID uuid.UUID) map[string]string {
	t.Helper()
	var agent model.Agent
	if err := h.db.First(&agent, "org_id = ?", orgID).Error; err != nil {
		t.Fatalf("load agent: %v", err)
	}
	if len(agent.EncryptedEnvVars) == 0 {
		return map[string]string{}
	}
	decrypted, err := encKey.DecryptString(agent.EncryptedEnvVars)
	if err != nil {
		t.Fatalf("decrypt env vars: %v", err)
	}
	var out map[string]string
	if err := json.Unmarshal([]byte(decrypted), &out); err != nil {
		t.Fatalf("decode env vars: %v", err)
	}
	return out
}

func saveOrgTestEnvVars(t *testing.T, h *orgUpdateHarness, encKey interface {
	EncryptString(string) ([]byte, error)
}, orgID uuid.UUID, envVars map[string]string) {
	t.Helper()
	encoded, err := json.Marshal(envVars)
	if err != nil {
		t.Fatalf("encode env vars: %v", err)
	}
	encrypted, err := encKey.EncryptString(string(encoded))
	if err != nil {
		t.Fatalf("encrypt env vars: %v", err)
	}
	if err := h.db.Model(&model.Agent{}).Where("org_id = ?", orgID).Update("encrypted_env_vars", encrypted).Error; err != nil {
		t.Fatalf("save env vars: %v", err)
	}
}
