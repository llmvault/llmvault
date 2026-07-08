package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"gorm.io/driver/sqlite"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/microsandbox/config"
	"github.com/usehivy/hivy/internal/microsandbox/model"
)

func TestValidateAlias(t *testing.T) {
	cases := []struct {
		name    string
		alias   string
		wantErr bool
	}{
		{"valid", "my-app", false},
		{"valid digits", "app123", false},
		{"too short", "ab", true},
		{"too long", strings.Repeat("a", 64), true},
		{"uppercase rejected by charset", "MyApp", true},
		{"underscore", "my_app", true},
		{"leading hyphen", "-app", true},
		{"trailing hyphen", "app-", true},
		{"port prefix collision", "3000-app", true},
		{"single digit port prefix", "8-app", true},
		{"reserved www", "www", true},
		{"reserved api", "api", true},
		{"reserved apps", "apps", true},
		{"reserved gateway", "gateway", true},
		{"reserved preview", "preview", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := validateAlias(tc.alias)
			if tc.wantErr && err == nil {
				t.Fatalf("validateAlias(%q) = nil, want error", tc.alias)
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("validateAlias(%q) = %v, want nil", tc.alias, err)
			}
		})
	}
}

type aliasGatewayCall struct {
	Method string
	Alias  string
}

// fakeAliasGateway records alias route pushes/deletes the control plane makes.
func fakeAliasGateway(t *testing.T, token string) (*httptest.Server, func() []aliasGatewayCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []aliasGatewayCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			t.Errorf("gateway auth = %q", r.Header.Get("Authorization"))
		}
		if !strings.HasPrefix(r.URL.Path, "/v1/aliases/") {
			t.Errorf("gateway path = %q, want /v1/aliases/*", r.URL.Path)
		}
		mu.Lock()
		calls = append(calls, aliasGatewayCall{Method: r.Method, Alias: strings.TrimPrefix(r.URL.Path, "/v1/aliases/")})
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	return srv, func() []aliasGatewayCall {
		mu.Lock()
		defer mu.Unlock()
		out := make([]aliasGatewayCall, len(calls))
		copy(out, calls)
		return out
	}
}

func newAliasTestServer(t *testing.T, gatewayURL, cacheToken string) *Server {
	t.Helper()
	db, err := gorm.Open(sqlite.Open("file:alias-"+t.Name()+"?mode=memory&cache=shared"), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	if err := db.AutoMigrate(&model.Runner{}, &model.Sandbox{}, &model.SandboxPort{}, &model.Alias{}); err != nil {
		t.Fatal(err)
	}
	cfg := config.Config{
		APIToken:          "api-token",
		PreviewBaseDomain: "preview.test",
		PreviewCacheURL:   gatewayURL,
		PreviewCacheToken: cacheToken,
	}
	s := &Server{db: db, cfg: cfg}
	if gatewayURL != "" {
		s.previewCache = NewPreviewCacheClient(context.Background(), cfg)
	}
	return s
}

func seedSandbox(t *testing.T, s *Server, id string) {
	t.Helper()
	seedSandboxForOrg(t, s, id, "org_1")
}

func seedSandboxForOrg(t *testing.T, s *Server, id, orgID string) {
	t.Helper()
	if err := s.db.Create(&model.Sandbox{
		ID: id, OrgID: orgID, RunnerID: "runner-1", Name: id, ImageRef: "img",
		Status: model.SandboxStatusRunning, CPU: 1, MemoryMB: 1024, DiskGB: 5,
	}).Error; err != nil {
		t.Fatal(err)
	}
}

func doAlias(s *Server, method, alias, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, "/v1/aliases/"+alias, strings.NewReader(body))
	req.Header.Set("Authorization", "Bearer api-token")
	if body != "" {
		req.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	s.Routes().ServeHTTP(rec, req)
	return rec
}

func TestAliasClaimRepointGetDelete(t *testing.T) {
	gw, calls := fakeAliasGateway(t, "cache-token")
	defer gw.Close()
	s := newAliasTestServer(t, gw.URL, "cache-token")
	seedSandbox(t, s, "sbx_one")
	seedSandbox(t, s, "sbx_two")

	// Claim.
	rec := doAlias(s, http.MethodPut, "my-app", `{"sandbox_id":"sbx_one","port":8080}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim status = %d body = %s", rec.Code, rec.Body.String())
	}
	var resp aliasResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp.URL != "https://my-app.preview.test" || resp.SandboxID != "sbx_one" || resp.Port != 8080 {
		t.Fatalf("claim response = %+v", resp)
	}

	// Repoint to a new sandbox (last-write-wins).
	rec = doAlias(s, http.MethodPut, "my-app", `{"sandbox_id":"sbx_two","port":9090}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("repoint status = %d body = %s", rec.Code, rec.Body.String())
	}
	var row model.Alias
	if err := s.db.First(&row, "alias = ?", "my-app").Error; err != nil {
		t.Fatal(err)
	}
	if row.SandboxID != "sbx_two" || row.Port != 9090 {
		t.Fatalf("after repoint row = %+v", row)
	}

	// Get.
	rec = doAlias(s, http.MethodGet, "my-app", "")
	if rec.Code != http.StatusOK {
		t.Fatalf("get status = %d", rec.Code)
	}

	// Delete.
	rec = doAlias(s, http.MethodDelete, "my-app", "")
	if rec.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d", rec.Code)
	}
	if err := s.db.First(&row, "alias = ?", "my-app").Error; err == nil {
		t.Fatal("alias row still present after delete")
	}

	// Two PUTs push, one DELETE removes.
	got := calls()
	if len(got) != 3 {
		t.Fatalf("gateway calls = %+v", got)
	}
	if got[0].Method != http.MethodPut || got[1].Method != http.MethodPut || got[2].Method != http.MethodDelete {
		t.Fatalf("gateway call methods = %+v", got)
	}
}

func TestAliasClaimRejectsInvalidAndMissingSandbox(t *testing.T) {
	s := newAliasTestServer(t, "", "")
	seedSandbox(t, s, "sbx_one")

	if rec := doAlias(s, http.MethodPut, "3000-app", `{"sandbox_id":"sbx_one","port":8080}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("port-prefix alias status = %d", rec.Code)
	}
	if rec := doAlias(s, http.MethodPut, "api", `{"sandbox_id":"sbx_one","port":8080}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("reserved alias status = %d", rec.Code)
	}
	if rec := doAlias(s, http.MethodPut, "good-app", `{"sandbox_id":"nope","port":8080}`); rec.Code != http.StatusNotFound {
		t.Fatalf("missing sandbox status = %d", rec.Code)
	}
	if rec := doAlias(s, http.MethodPut, "good-app", `{"sandbox_id":"sbx_one","port":0}`); rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid port status = %d", rec.Code)
	}
}

// TestAliasClaimRejectsCrossOrgRepoint asserts an alias owned by one org cannot
// be repointed by a sandbox belonging to another org — no hostname takeover.
func TestAliasClaimRejectsCrossOrgRepoint(t *testing.T) {
	gw, _ := fakeAliasGateway(t, "cache-token")
	defer gw.Close()
	s := newAliasTestServer(t, gw.URL, "cache-token")
	seedSandboxForOrg(t, s, "sbx_a", "org_a")
	seedSandboxForOrg(t, s, "sbx_b", "org_b")

	// Org A claims the slug.
	if rec := doAlias(s, http.MethodPut, "shared-slug", `{"sandbox_id":"sbx_a","port":8080}`); rec.Code != http.StatusOK {
		t.Fatalf("org A claim status = %d body = %s", rec.Code, rec.Body.String())
	}

	// Org B trying to repoint the same hostname must be rejected.
	rec := doAlias(s, http.MethodPut, "shared-slug", `{"sandbox_id":"sbx_b","port":9090}`)
	if rec.Code != http.StatusConflict {
		t.Fatalf("cross-org repoint status = %d, want 409; body = %s", rec.Code, rec.Body.String())
	}

	// The mapping still points at Org A's sandbox.
	var row model.Alias
	if err := s.db.First(&row, "alias = ?", "shared-slug").Error; err != nil {
		t.Fatal(err)
	}
	if row.SandboxID != "sbx_a" || row.OrgID != "org_a" {
		t.Fatalf("alias was hijacked: %+v", row)
	}
}

func TestAliasDeleteUnknownIsNoContent(t *testing.T) {
	s := newAliasTestServer(t, "", "")
	if rec := doAlias(s, http.MethodDelete, "ghost-app", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete unknown status = %d", rec.Code)
	}
}
