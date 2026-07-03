package apps

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// appdRecord captures one request the fake appd received.
type appdRecord struct {
	Method string
	Path   string
	Bearer string
	Body   map[string]any
}

// fakeAppd is an httptest-backed hivy-appd double shared by the publish,
// deploy, and MCP tool tests.
type fakeAppd struct {
	mu       sync.Mutex
	records  []appdRecord
	server   *httptest.Server
	statuses map[string]int // path → forced status (default 200)
	logs     []string       // lines served by GET /logs (default canned pair)
}

func (f *fakeAppd) logLines() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.logs != nil {
		return f.logs
	}
	return []string{`{"level":"info","msg":"server listening"}`, `{"level":"error","msg":"boom"}`}
}

func newFakeAppd(t *testing.T) *fakeAppd {
	t.Helper()
	f := &fakeAppd{statuses: map[string]int{}}
	f.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		payload, _ := io.ReadAll(r.Body)
		record := appdRecord{Method: r.Method, Path: r.URL.Path, Bearer: bearerToken(r)}
		if len(payload) > 0 {
			_ = json.Unmarshal(payload, &record.Body)
		}
		f.mu.Lock()
		f.records = append(f.records, record)
		status := f.statuses[r.URL.Path]
		f.mu.Unlock()
		if status == 0 {
			status = http.StatusOK
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(status)
		switch r.URL.Path {
		case "/deploy", "/rollback":
			_ = json.NewEncoder(w).Encode(map[string]any{"new_sha": "ok"})
		case "/health":
			// Mirrors cmd/hivy-appd healthResponse.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"ok":  status == http.StatusOK,
				"app": map[string]any{"mode": "direct", "state": "running", "pid": 42},
			})
		case "/logs":
			// Mirrors cmd/hivy-appd logsResponse.
			_ = json.NewEncoder(w).Encode(map[string]any{
				"stream": "app",
				"lines":  f.logLines(),
				"count":  len(f.logLines()),
				"app":    map[string]any{"mode": "direct", "state": "running", "pid": 42},
			})
		default:
			_ = json.NewEncoder(w).Encode(map[string]any{"ok": status == http.StatusOK, "error": "forced"})
		}
	}))
	t.Cleanup(f.server.Close)
	return f
}

func bearerToken(r *http.Request) string {
	const prefix = "Bearer "
	header := r.Header.Get("Authorization")
	if len(header) > len(prefix) {
		return header[len(prefix):]
	}
	return ""
}

// forceStatus makes path reply with the given HTTP status; safe to call while
// the server is live (the handler reads statuses under the same mutex).
func (f *fakeAppd) forceStatus(path string, status int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.statuses[path] = status
}

func (f *fakeAppd) recorded(path string) []appdRecord {
	f.mu.Lock()
	defer f.mu.Unlock()
	var out []appdRecord
	for _, record := range f.records {
		if record.Path == path {
			out = append(out, record)
		}
	}
	return out
}

func seedChannelEnvVar(t *testing.T, h *appsTestHarness, name, value string) {
	t.Helper()
	encrypted, err := h.encKey.EncryptString(value)
	if err != nil {
		t.Fatalf("encrypt channel env var: %v", err)
	}
	envVar := model.ChannelEnvVar{OrgID: h.org.ID, ChannelID: h.channel.ID, Name: name, EncryptedValue: encrypted}
	if err := h.db.Create(&envVar).Error; err != nil {
		t.Fatalf("create channel env var: %v", err)
	}
}
