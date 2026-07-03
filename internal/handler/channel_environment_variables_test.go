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

type channelEnvHarness struct {
	db        *gorm.DB
	router    *chi.Mux
	key       *crypto.SymmetricKey
	fx        channelFixture
	channelID string
}

func newChannelEnvKey(t *testing.T) *crypto.SymmetricKey {
	t.Helper()
	key, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(make([]byte, 32)))
	if err != nil {
		t.Fatalf("create enc key: %v", err)
	}
	return key
}

func newChannelEnvHarness(t *testing.T) *channelEnvHarness {
	t.Helper()
	db := connectTestDB(t)
	key := newChannelEnvKey(t)
	h := handler.NewChannelHandler(db, handler.WithChannelEnvEncryptionKey(key))
	r := chi.NewRouter()
	r.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Use(middleware.RequireAPIKeyScopeOrJWT("channels"))
		r.Post("/channels", h.Create)
		r.Get("/channels/{id}/environment-variables", h.ListChannelEnvironmentVariables)
		r.Post("/channels/{id}/environment-variables", h.CreateChannelEnvironmentVariable)
		r.Patch("/channels/{id}/environment-variables/{name}", h.UpdateChannelEnvironmentVariable)
		r.Delete("/channels/{id}/environment-variables/{name}", h.DeleteChannelEnvironmentVariable)
	})

	seeder := &channelHarness{db: db}
	fx := seeder.seed(t)

	harness := &channelEnvHarness{db: db, router: r, key: key, fx: fx}
	rr := harness.do(t, fx.owner, http.MethodPost, "/v1/channels", map[string]any{
		"name":             "#engineering",
		"default_agent_id": fx.agent.ID.String(),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create channel: status=%d body=%s", rr.Code, rr.Body.String())
	}
	harness.channelID = decodeChannelCreate(t, rr).Channel.ID
	return harness
}

func (h *channelEnvHarness) do(t *testing.T, user model.User, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", h.fx.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: user.ID.String(),
		OrgID:  h.fx.org.ID.String(),
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *channelEnvHarness) envPath() string {
	return "/v1/channels/" + h.channelID + "/environment-variables"
}

type envVarNames struct {
	Data []struct {
		Name string `json:"name"`
	} `json:"data"`
}

func (h *channelEnvHarness) listNames(t *testing.T) []string {
	t.Helper()
	rr := h.do(t, h.fx.owner, http.MethodGet, h.envPath(), nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list: status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out envVarNames
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	names := make([]string, 0, len(out.Data))
	for _, d := range out.Data {
		names = append(names, d.Name)
	}
	return names
}

func TestChannelEnvVars_CRUD(t *testing.T) {
	h := newChannelEnvHarness(t)

	// Create.
	rr := h.do(t, h.fx.owner, http.MethodPost, h.envPath(), map[string]any{"name": "database_url", "value": "postgres://a"})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create: status=%d body=%s", rr.Code, rr.Body.String())
	}

	// The value is encrypted at rest and decrypts back to the plaintext.
	var stored model.ChannelEnvVar
	if err := h.db.Where("channel_id = ? AND name = ?", h.channelID, "DATABASE_URL").First(&stored).Error; err != nil {
		t.Fatalf("load stored var: %v", err)
	}
	if string(stored.EncryptedValue) == "postgres://a" || len(stored.EncryptedValue) == 0 {
		t.Fatalf("value not encrypted at rest: %q", string(stored.EncryptedValue))
	}
	if dec, err := h.key.DecryptString(stored.EncryptedValue); err != nil || dec != "postgres://a" {
		t.Fatalf("decrypt stored value = %q, err=%v; want postgres://a", dec, err)
	}

	// List returns the name (uppercased) and never the value.
	names := h.listNames(t)
	if len(names) != 1 || names[0] != "DATABASE_URL" {
		t.Fatalf("list names = %v, want [DATABASE_URL]", names)
	}
	if body := h.do(t, h.fx.owner, http.MethodGet, h.envPath(), nil).Body.String(); bytesContains(body, "postgres://a") {
		t.Fatalf("list leaked value: %s", body)
	}

	// Duplicate name → 409.
	if rr := h.do(t, h.fx.owner, http.MethodPost, h.envPath(), map[string]any{"name": "DATABASE_URL", "value": "x"}); rr.Code != http.StatusConflict {
		t.Fatalf("duplicate create: status=%d, want 409", rr.Code)
	}

	// Update value.
	if rr := h.do(t, h.fx.owner, http.MethodPatch, h.envPath()+"/DATABASE_URL", map[string]any{"value": "postgres://b"}); rr.Code != http.StatusOK {
		t.Fatalf("update value: status=%d body=%s", rr.Code, rr.Body.String())
	}
	_ = h.db.Where("channel_id = ? AND name = ?", h.channelID, "DATABASE_URL").First(&stored)
	if dec, _ := h.key.DecryptString(stored.EncryptedValue); dec != "postgres://b" {
		t.Fatalf("updated value = %q, want postgres://b", dec)
	}

	// Rename.
	if rr := h.do(t, h.fx.owner, http.MethodPatch, h.envPath()+"/DATABASE_URL", map[string]any{"name": "db_url"}); rr.Code != http.StatusOK {
		t.Fatalf("rename: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if names := h.listNames(t); len(names) != 1 || names[0] != "DB_URL" {
		t.Fatalf("after rename names = %v, want [DB_URL]", names)
	}

	// Rename collision → 409.
	if rr := h.do(t, h.fx.owner, http.MethodPost, h.envPath(), map[string]any{"name": "OTHER", "value": "y"}); rr.Code != http.StatusCreated {
		t.Fatalf("create OTHER: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := h.do(t, h.fx.owner, http.MethodPatch, h.envPath()+"/OTHER", map[string]any{"name": "DB_URL"}); rr.Code != http.StatusConflict {
		t.Fatalf("rename collision: status=%d, want 409", rr.Code)
	}

	// Delete.
	if rr := h.do(t, h.fx.owner, http.MethodDelete, h.envPath()+"/DB_URL", nil); rr.Code != http.StatusOK {
		t.Fatalf("delete: status=%d body=%s", rr.Code, rr.Body.String())
	}
	// Delete missing → 404.
	if rr := h.do(t, h.fx.owner, http.MethodDelete, h.envPath()+"/DB_URL", nil); rr.Code != http.StatusNotFound {
		t.Fatalf("delete missing: status=%d, want 404", rr.Code)
	}
	// Update missing → 404.
	if rr := h.do(t, h.fx.owner, http.MethodPatch, h.envPath()+"/NOPE", map[string]any{"value": "z"}); rr.Code != http.StatusNotFound {
		t.Fatalf("update missing: status=%d, want 404", rr.Code)
	}
}

func TestChannelEnvVars_NameValidation(t *testing.T) {
	h := newChannelEnvHarness(t)

	cases := []struct {
		name string
		want int
	}{
		{"__ENV__FOO", http.StatusBadRequest},
		{"HIVY_FOO", http.StatusBadRequest},
		{"1BAD", http.StatusBadRequest},
		{"has-dash", http.StatusBadRequest},
		{"", http.StatusBadRequest},
		{"lower_ok", http.StatusCreated}, // normalized to LOWER_OK
	}
	for _, tc := range cases {
		rr := h.do(t, h.fx.owner, http.MethodPost, h.envPath(), map[string]any{"name": tc.name, "value": "v"})
		if rr.Code != tc.want {
			t.Fatalf("name %q: status=%d, want %d (body=%s)", tc.name, rr.Code, tc.want, rr.Body.String())
		}
	}
	if names := h.listNames(t); len(names) != 1 || names[0] != "LOWER_OK" {
		t.Fatalf("names = %v, want [LOWER_OK]", names)
	}
}

func TestChannelEnvVars_MissingValue(t *testing.T) {
	h := newChannelEnvHarness(t)
	if rr := h.do(t, h.fx.owner, http.MethodPost, h.envPath(), map[string]any{"name": "FOO"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("missing value: status=%d, want 400", rr.Code)
	}
}

func bytesContains(haystack, needle string) bool {
	return len(needle) > 0 && bytes.Contains([]byte(haystack), []byte(needle))
}
