package control

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
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
	if err := s.db.Create(&model.Sandbox{
		ID: id, OrgID: "org_1", RunnerID: "runner-1", Name: id, ImageRef: "img",
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

func TestAliasDeleteUnknownIsNoContent(t *testing.T) {
	s := newAliasTestServer(t, "", "")
	if rec := doAlias(s, http.MethodDelete, "ghost-app", ""); rec.Code != http.StatusNoContent {
		t.Fatalf("delete unknown status = %d", rec.Code)
	}
}

func TestSyncAndDeleteSandboxAliasRoutes(t *testing.T) {
	gw, calls := fakeAliasGateway(t, "cache-token")
	defer gw.Close()
	s := newAliasTestServer(t, gw.URL, "cache-token")
	seedSandbox(t, s, "sbx_live")
	if err := s.db.Create(&model.Alias{Alias: "app-a", SandboxID: "sbx_live", Port: 8080}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&model.Alias{Alias: "app-b", SandboxID: "sbx_live", Port: 3000}).Error; err != nil {
		t.Fatal(err)
	}

	// Wake/stop refresh path pushes every alias for the sandbox.
	s.syncSandboxAliasRoutes(context.Background(), "sbx_live")
	if got := calls(); len(got) != 2 || got[0].Method != http.MethodPut || got[1].Method != http.MethodPut {
		t.Fatalf("refresh calls = %+v", got)
	}

	// Delete path removes gateway entries but keeps the DB mapping (dead-sandbox
	// mapping survives for a later repoint).
	s.deleteSandboxAliasRoutes(context.Background(), "sbx_live")
	del := calls()[2:]
	if len(del) != 2 || del[0].Method != http.MethodDelete || del[1].Method != http.MethodDelete {
		t.Fatalf("delete calls = %+v", del)
	}
	var count int64
	if err := s.db.Model(&model.Alias{}).Where("sandbox_id = ?", "sbx_live").Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("alias rows after sandbox alias delete = %d, want 2", count)
	}
}

// TestClaimAliasFailsWhenGatewayPushFails proves the claim fails closed: the
// old behavior returned 200 + URL even when the gateway never got the mapping,
// stranding the alias behind "invalid preview host".
func TestClaimAliasFailsWhenGatewayPushFails(t *testing.T) {
	var fail atomic.Bool
	fail.Store(true)
	gw := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "gateway down", http.StatusInternalServerError)
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	defer gw.Close()
	s := newAliasTestServer(t, gw.URL, "cache-token")
	seedSandbox(t, s, "sbx_one")

	// Gateway push fails → claim must return non-2xx, not 200 + URL.
	rec := doAlias(s, http.MethodPut, "my-app", `{"sandbox_id":"sbx_one","port":8080}`)
	if rec.Code != http.StatusBadGateway {
		t.Fatalf("claim status = %d, want 502 when gateway push fails; body = %s", rec.Code, rec.Body.String())
	}

	// Gateway recovers → claim succeeds with 200.
	fail.Store(false)
	rec = doAlias(s, http.MethodPut, "my-app", `{"sandbox_id":"sbx_one","port":8080}`)
	if rec.Code != http.StatusOK {
		t.Fatalf("claim status = %d, want 200 when gateway push succeeds; body = %s", rec.Code, rec.Body.String())
	}
}

type gatewayCall struct {
	Method string
	Path   string
}

// recordingGateway accepts both alias and route pushes and records their paths.
func recordingGateway(t *testing.T) (*httptest.Server, func() []gatewayCall) {
	t.Helper()
	var mu sync.Mutex
	var calls []gatewayCall
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		calls = append(calls, gatewayCall{Method: r.Method, Path: r.URL.Path})
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"ok": true})
	}))
	return srv, func() []gatewayCall {
		mu.Lock()
		defer mu.Unlock()
		out := make([]gatewayCall, len(calls))
		copy(out, calls)
		return out
	}
}

// TestClaimAliasResyncsPreviewRouteOnRedeploy proves a redeploy into an existing
// sandbox (a second claim of the same alias) re-pushes the sandbox's preview
// route to the gateway, healing a route whose gateway TTL lapsed.
func TestClaimAliasResyncsPreviewRouteOnRedeploy(t *testing.T) {
	gw, calls := recordingGateway(t)
	defer gw.Close()
	s := newAliasTestServer(t, gw.URL, "cache-token")

	// A live runner + sandbox + port so a preview route can actually be built.
	if err := s.db.Create(&model.Runner{
		ID: "runner-1", Name: "r1", APIURL: "http://runner", PreviewBaseURL: "http://10.0.0.9",
		AuthTokenHash: []byte("x"), Status: model.RunnerStatusHealthy,
		TotalCPU: 8, TotalMemoryMB: 8192, TotalDiskGB: 100,
	}).Error; err != nil {
		t.Fatal(err)
	}
	seedSandbox(t, s, "sbx_one")
	if err := s.db.Create(&model.SandboxPort{
		ID: "port-1", SandboxID: "sbx_one", GuestPort: 8080, HostPort: 43080, Protocol: "http",
	}).Error; err != nil {
		t.Fatal(err)
	}

	for i := 0; i < 2; i++ {
		rec := doAlias(s, http.MethodPut, "my-app", `{"sandbox_id":"sbx_one","port":8080}`)
		if rec.Code != http.StatusOK {
			t.Fatalf("claim %d status = %d body = %s", i, rec.Code, rec.Body.String())
		}
	}

	var aliasPuts, routePuts int
	for _, c := range calls() {
		switch {
		case c.Method == http.MethodPut && strings.HasPrefix(c.Path, "/v1/aliases/"):
			aliasPuts++
		case c.Method == http.MethodPut && strings.HasPrefix(c.Path, "/v1/routes/"):
			routePuts++
		}
	}
	if aliasPuts != 2 {
		t.Fatalf("alias route puts = %d, want 2", aliasPuts)
	}
	if routePuts != 2 {
		t.Fatalf("preview route puts = %d, want 2 (route re-synced on every claim/redeploy)", routePuts)
	}
}

func TestBulkSyncAliasRoutesSkipsDeadSandboxes(t *testing.T) {
	gw, calls := fakeAliasGateway(t, "cache-token")
	defer gw.Close()
	s := newAliasTestServer(t, gw.URL, "cache-token")
	seedSandbox(t, s, "sbx_live")
	// Alias pointing at a live sandbox and one pointing at a since-deleted sandbox.
	if err := s.db.Create(&model.Alias{Alias: "live-app", SandboxID: "sbx_live", Port: 8080}).Error; err != nil {
		t.Fatal(err)
	}
	if err := s.db.Create(&model.Alias{Alias: "dead-app", SandboxID: "sbx_gone", Port: 8080}).Error; err != nil {
		t.Fatal(err)
	}

	s.bulkSyncAliasRoutes(context.Background(), map[string]bool{"sbx_live": true})

	got := calls()
	if len(got) != 1 || got[0].Alias != "bulk" || got[0].Method != http.MethodPost {
		t.Fatalf("bulk alias sync calls = %+v", got)
	}
}
