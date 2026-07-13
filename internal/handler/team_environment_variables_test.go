package handler_test

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type teamEnvHarness struct {
	db     *gorm.DB
	router *chi.Mux
	key    *crypto.SymmetricKey
	fx     channelFixture
}

func newTeamEnvHarness(t *testing.T) *teamEnvHarness {
	t.Helper()
	db := connectTestDB(t)
	key, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatalf("create encryption key: %v", err)
	}
	h := handler.NewTeamHandler(db, handler.WithTeamEnvEncryptionKey(key))
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("teams"))
		r.Get("/orgs/current/teams/{id}/environment-variables", h.ListEnvironmentVariables)
		r.Post("/orgs/current/teams/{id}/environment-variables", h.CreateEnvironmentVariable)
		r.Patch("/orgs/current/teams/{id}/environment-variables/{name}", h.UpdateEnvironmentVariable)
		r.Delete("/orgs/current/teams/{id}/environment-variables/{name}", h.DeleteEnvironmentVariable)
	})
	fx := (&channelHarness{db: db}).seed(t)
	return &teamEnvHarness{db: db, router: r, key: key, fx: fx}
}

func (h *teamEnvHarness) do(t *testing.T, user model.User, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			t.Fatalf("encode request: %v", err)
		}
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", h.fx.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: user.ID.String(), OrgID: h.fx.org.ID.String()})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *teamEnvHarness) path() string {
	return "/v1/orgs/current/teams/" + h.fx.team.ID.String() + "/environment-variables"
}

func TestTeamEnvironmentVariablesCRUD(t *testing.T) {
	h := newTeamEnvHarness(t)
	path := h.path()

	created := h.do(t, h.fx.owner, http.MethodPost, path, map[string]any{
		"name": "database_url", "value": "postgres://a", "description": "  Primary database. ",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", created.Code, created.Body.String())
	}

	var stored model.TeamEnvVar
	if err := h.db.Where("org_id = ? AND team_id = ? AND name = ?", h.fx.org.ID, h.fx.team.ID, "DATABASE_URL").First(&stored).Error; err != nil {
		t.Fatalf("load stored variable: %v", err)
	}
	if string(stored.EncryptedValue) == "postgres://a" || len(stored.EncryptedValue) == 0 {
		t.Fatalf("value was not encrypted at rest")
	}
	if value, err := h.key.DecryptString(stored.EncryptedValue); err != nil || value != "postgres://a" {
		t.Fatalf("decrypt stored value = %q, err=%v", value, err)
	}
	if stored.Description != "Primary database." {
		t.Fatalf("description = %q", stored.Description)
	}

	listed := h.do(t, h.fx.owner, http.MethodGet, path, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("list: status=%d body=%s", listed.Code, listed.Body.String())
	}
	if !bytes.Contains(listed.Body.Bytes(), []byte(`"name":"DATABASE_URL"`)) ||
		bytes.Contains(listed.Body.Bytes(), []byte("postgres://a")) {
		t.Fatalf("unexpected list response: %s", listed.Body.String())
	}

	updated := h.do(t, h.fx.owner, http.MethodPatch, path+"/DATABASE_URL", map[string]any{
		"name": "db_url", "value": "postgres://b", "description": "Replica database.",
	})
	if updated.Code != http.StatusOK {
		t.Fatalf("update: status=%d body=%s", updated.Code, updated.Body.String())
	}
	if err := h.db.Where("org_id = ? AND team_id = ? AND name = ?", h.fx.org.ID, h.fx.team.ID, "DB_URL").First(&stored).Error; err != nil {
		t.Fatalf("load updated variable: %v", err)
	}
	if value, err := h.key.DecryptString(stored.EncryptedValue); err != nil || value != "postgres://b" {
		t.Fatalf("decrypt updated value = %q, err=%v", value, err)
	}

	deleted := h.do(t, h.fx.owner, http.MethodDelete, path+"/DB_URL", nil)
	if deleted.Code != http.StatusOK || !bytes.Contains(deleted.Body.Bytes(), []byte(`"status":"deleted"`)) {
		t.Fatalf("delete: status=%d body=%s", deleted.Code, deleted.Body.String())
	}
}

func TestTeamEnvironmentVariablesRejectDuplicateAndInvalidNames(t *testing.T) {
	h := newTeamEnvHarness(t)
	path := h.path()
	if rr := h.do(t, h.fx.owner, http.MethodPost, path, map[string]any{"name": "TOKEN", "value": "one"}); rr.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := h.do(t, h.fx.owner, http.MethodPost, path, map[string]any{"name": "token", "value": "two"}); rr.Code != http.StatusConflict {
		t.Fatalf("duplicate: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := h.do(t, h.fx.owner, http.MethodPost, path, map[string]any{"name": "HIVY_TOKEN", "value": "two"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("reserved name: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTeamEnvironmentVariablesHideInaccessibleTeam(t *testing.T) {
	h := newTeamEnvHarness(t)
	rr := h.do(t, h.fx.member, http.MethodGet, h.path(), nil)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("non-member list: status=%d body=%s", rr.Code, rr.Body.String())
	}
}
