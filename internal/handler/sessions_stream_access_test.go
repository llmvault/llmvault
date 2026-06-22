package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SandboxAccessAllowsVisibleReadOnlySessionAndMintsJWT(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Inspect repo files")
	runtimeSecret := "runtime-repo-secret"
	encSecret, err := sessionTestEncKey(t).EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sb := model.Sandbox{
		OrgID:                  &fx.org.ID,
		AgentID:                &fx.agent.ID,
		ProviderID:             "docker",
		ExternalID:             "sandbox-access-container",
		RuntimeURL:             "http://203.0.113.10:7080",
		EncryptedRuntimeSecret: encSecret,
		Status:                 "running",
	}
	if err := h.db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	if err := h.db.Model(&model.Session{}).Where("id = ?", created.Session.ID).Update("sandbox_id", sb.ID).Error; err != nil {
		t.Fatalf("attach sandbox: %v", err)
	}
	path := "/v1/sessions/" + created.Session.ID + "/sandbox-access"
	blocked := h.doJSON(t, http.MethodPost, path, fx, fx.member, nil)
	if blocked.Code != http.StatusForbidden {
		t.Fatalf("unshared member sandbox access status=%d body=%s", blocked.Code, blocked.Body.String())
	}

	if err := h.db.Create(&model.ChannelMember{ChannelID: fx.channel.ID, UserID: fx.viewer.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add viewer to channel: %v", err)
	}
	sandboxAccess := h.doJSON(t, http.MethodPost, path, fx, fx.viewer, nil)
	if sandboxAccess.Code != http.StatusOK {
		t.Fatalf("sandbox access status=%d body=%s", sandboxAccess.Code, sandboxAccess.Body.String())
	}
	var access struct {
		SessionID      string   `json:"session_id"`
		SandboxID      string   `json:"sandbox_id"`
		SandboxBaseURL string   `json:"sandbox_base_url"`
		Token          string   `json:"token"`
		ExpiresAt      string   `json:"expires_at"`
		Scopes         []string `json:"scopes"`
	}
	if err := json.Unmarshal(sandboxAccess.Body.Bytes(), &access); err != nil {
		t.Fatalf("decode sandbox access: %v\n%s", err, sandboxAccess.Body.String())
	}
	if access.SandboxBaseURL != "http://203.0.113.10:7080" || access.SandboxID != sb.ID.String() {
		t.Fatalf("bad sandbox access target: %+v", access)
	}
	if !hasScopes(access.Scopes, "sandbox:read", "stream:read", "repo:read") {
		t.Fatalf("bad sandbox scopes: %+v", access.Scopes)
	}
	claims := jwt.MapClaims{}
	parsed, err := jwt.ParseWithClaims(access.Token, claims, func(token *jwt.Token) (any, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			t.Fatalf("unexpected alg: %s", token.Method.Alg())
		}
		return []byte(runtimeSecret), nil
	}, jwt.WithAudience("hivy-runtime"), jwt.WithExpirationRequired())
	if err != nil || !parsed.Valid {
		t.Fatalf("sandbox jwt invalid: parsed=%v err=%v", parsed != nil && parsed.Valid, err)
	}
	if claims["session_id"] != created.Session.ID || claims["sandbox_id"] != sb.ID.String() || claims["org_id"] != fx.org.ID.String() || claims["sub"] != fx.viewer.ID.String() {
		t.Fatalf("bad sandbox jwt claims: %+v", claims)
	}
	rawScopes, ok := claims["scopes"].([]any)
	if !ok {
		t.Fatalf("jwt scopes=%#v, want array", claims["scopes"])
	}
	claimScopes := make([]string, 0, len(rawScopes))
	for _, scope := range rawScopes {
		if value, ok := scope.(string); ok {
			claimScopes = append(claimScopes, value)
		}
	}
	if !hasScopes(claimScopes, "sandbox:read", "stream:read", "repo:read") {
		t.Fatalf("bad jwt scopes: %+v", claimScopes)
	}
}

func hasScopes(got []string, want ...string) bool {
	seen := map[string]bool{}
	for _, scope := range got {
		seen[scope] = true
	}
	for _, scope := range want {
		if !seen[scope] {
			return false
		}
	}
	return true
}
