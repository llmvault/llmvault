package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/golang-jwt/jwt/v5"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SandboxAccessRequiresParticipantAndMintsJWT(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.owner, "Stream this turn")
	runtimeSecret := "runtime-stream-secret"
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

	invite := h.doJSON(t, http.MethodPut, "/v1/sessions/"+created.Session.ID+"/participants/"+fx.member.ID.String(), fx, fx.owner, nil)
	if invite.Code != http.StatusOK {
		t.Fatalf("invite status=%d body=%s", invite.Code, invite.Body.String())
	}
	sandboxAccess := h.doJSON(t, http.MethodPost, path, fx, fx.member, nil)
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
	if len(access.Scopes) != 2 || access.Scopes[0] != "stream:read" || access.Scopes[1] != "repo:read" {
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
	if claims["session_id"] != created.Session.ID || claims["sandbox_id"] != sb.ID.String() {
		t.Fatalf("bad sandbox jwt claims: %+v", claims)
	}
}
