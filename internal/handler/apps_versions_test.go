package handler_test

import (
	"archive/zip"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// appdPort / appPort mirror the internal apps constants used by Deploy.
const (
	testAppdPort = 7080
	testAppPort  = 8080
)

// testAppd is a minimal hivy-appd double that records POST /deploy bodies.
type testAppd struct {
	server *httptest.Server
	mu     sync.Mutex
	deploy []map[string]any
}

func newTestAppd(t *testing.T) *testAppd {
	t.Helper()
	a := &testAppd{}
	a.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/deploy" {
			var body map[string]any
			raw, _ := io.ReadAll(r.Body)
			_ = json.Unmarshal(raw, &body)
			a.mu.Lock()
			a.deploy = append(a.deploy, body)
			a.mu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]any{"new_sha": "ok"})
	}))
	t.Cleanup(a.server.Close)
	return a
}

func (a *testAppd) deploys() []map[string]any {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.deploy
}

func makeTestZip(t *testing.T, name, content string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	f, err := zw.Create(name)
	if err != nil {
		t.Fatalf("zip create: %v", err)
	}
	if _, err := f.Write([]byte(content)); err != nil {
		t.Fatalf("zip write: %v", err)
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// doMultipart posts a multipart form with the given file parts + string fields,
// injecting org (+ optional user / API-key) context like the JSON `do` helper.
func (h *appsRESTHarness) doMultipart(t *testing.T, path string, files map[string][]byte, fields map[string]string, withUser bool) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for name, data := range files {
		fw, err := mw.CreateFormFile(name, name+".zip")
		if err != nil {
			t.Fatalf("create form file: %v", err)
		}
		if _, err := fw.Write(data); err != nil {
			t.Fatalf("write form file: %v", err)
		}
	}
	for name, value := range fields {
		if err := mw.WriteField(name, value); err != nil {
			t.Fatalf("write field: %v", err)
		}
	}
	if err := mw.Close(); err != nil {
		t.Fatalf("close multipart: %v", err)
	}

	req := httptest.NewRequest(http.MethodPost, path, &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req = middleware.WithOrg(req, &h.org)
	if withUser {
		req = middleware.WithUser(req, &h.user)
	} else {
		req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{OrgID: h.org.ID.String()})
	}
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestAppsVersionsPublishesAndDeploys(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "Publish REST")
	h.markDeployed(t, appID) // pre-create the sandbox so Deploy reuses it

	appd := newTestAppd(t)
	h.provider.endpoints[testAppdPort] = appd.server.URL
	h.provider.endpoints[testAppPort] = "http://127.0.0.1:45999"

	source := makeTestZip(t, "source.txt", "the source tree")
	bundle := makeTestZip(t, "index.js", "the deployable build")
	wantSourceSHA := sha256Hex(source)
	wantBundleSHA := sha256Hex(bundle)

	rr := h.doMultipart(t, "/v1/apps/"+appID+"/versions",
		map[string][]byte{"source": source, "bundle": bundle},
		map[string]string{"notes": "rest release"}, true)
	if rr.Code != http.StatusCreated {
		t.Fatalf("publish status = %d body=%s", rr.Code, rr.Body.String())
	}

	var resp struct {
		VersionID string `json:"version_id"`
		URL       string `json:"url"`
		Status    string `json:"status"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Status != model.AppStatusRunning {
		t.Fatalf("status = %q, want running", resp.Status)
	}
	if resp.URL != "http://127.0.0.1:45999" {
		t.Fatalf("url = %q", resp.URL)
	}

	// The version row carries the SERVER-computed shas and the immutable keys.
	var version model.AppVersion
	if err := h.db.Where("id = ?", resp.VersionID).First(&version).Error; err != nil {
		t.Fatalf("load version: %v", err)
	}
	var app model.App
	if err := h.db.Where("id = ?", appID).First(&app).Error; err != nil {
		t.Fatalf("load app: %v", err)
	}
	if version.SourceSHA256 != wantSourceSHA || version.BundleSHA256 != wantBundleSHA {
		t.Fatalf("version shas = %q / %q", version.SourceSHA256, version.BundleSHA256)
	}
	if version.SourceBytes != int64(len(source)) || version.BundleBytes != int64(len(bundle)) {
		t.Fatalf("version bytes = %d / %d", version.SourceBytes, version.BundleBytes)
	}
	if version.Notes != "rest release" {
		t.Fatalf("notes = %q", version.Notes)
	}
	wantSourceKey := fmt.Sprintf("pub/o/%s/apps/%s/%s/source.zip", h.org.ID, app.Slug, wantBundleSHA)
	wantBundleKey := fmt.Sprintf("pub/o/%s/apps/%s/%s/bundle.zip", h.org.ID, app.Slug, wantBundleSHA)
	if version.SourceObjectKey != wantSourceKey || version.BundleObjectKey != wantBundleKey {
		t.Fatalf("version keys = %q / %q", version.SourceObjectKey, version.BundleObjectKey)
	}

	// The objects actually landed at those content-addressed keys, intact.
	if data, ok := h.store.get(wantSourceKey); !ok || !bytes.Equal(data, source) {
		t.Fatalf("source object missing/mismatched at %s", wantSourceKey)
	}
	if data, ok := h.store.get(wantBundleKey); !ok || !bytes.Equal(data, bundle) {
		t.Fatalf("bundle object missing/mismatched at %s", wantBundleKey)
	}

	// Deploy was invoked with the version's bundle sha.
	deploys := appd.deploys()
	if len(deploys) != 1 {
		t.Fatalf("deploy calls = %d, want 1", len(deploys))
	}
	if deploys[0]["sha256"] != wantBundleSHA {
		t.Fatalf("deploy sha256 = %v, want %s", deploys[0]["sha256"], wantBundleSHA)
	}
	if deploys[0]["version_id"] != version.ID.String() {
		t.Fatalf("deploy version_id = %v", deploys[0]["version_id"])
	}
}

func TestAppsVersionsRejectsOversize(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "Oversize")
	restore := handler.SetAppVersionSizeCapsForTest(8, 8) // 8-byte caps
	defer restore()

	rr := h.doMultipart(t, "/v1/apps/"+appID+"/versions",
		map[string][]byte{
			"source": []byte("small"),
			"bundle": makeTestZip(t, "index.js", "way bigger than eight bytes"),
		}, nil, true)
	if rr.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("oversize status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAppsVersionsRejectsAPIKey(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "No API Key Publish")

	rr := h.doMultipart(t, "/v1/apps/"+appID+"/versions",
		map[string][]byte{"source": []byte("s"), "bundle": []byte("b")}, nil, false)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("api-key publish status = %d body=%s", rr.Code, rr.Body.String())
	}
}

func TestAppsVersionsChannelDenied404(t *testing.T) {
	h := newAppsRESTHarness(t)
	appID := h.createAppViaAPI(t, "Team Locked Publish")

	// Restrict the app's channel to a team the member is not in — the app is
	// then indistinguishable from a missing one (404).
	team := model.Team{OrgID: h.org.ID, Name: "publish-denied-" + h.org.ID.String()[:8]}
	if err := h.db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	t.Cleanup(func() { h.db.Delete(&model.Team{}, "id = ?", team.ID) })
	if err := h.db.Model(&model.Channel{}).Where("id = ?", h.channel.ID).
		Update("team_id", team.ID).Error; err != nil {
		t.Fatalf("restrict channel: %v", err)
	}

	rr := h.doMultipart(t, "/v1/apps/"+appID+"/versions",
		map[string][]byte{"source": []byte("s"), "bundle": []byte("b")}, nil, true)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("wrong-channel publish status = %d body=%s", rr.Code, rr.Body.String())
	}
}

// sha256Hex is a test helper matching the server's digest encoding.
func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
