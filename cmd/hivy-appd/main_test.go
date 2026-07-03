package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

const testSecret = "hvapp_test_secret_0123456789abcdef" // #nosec G101 -- test fixture, not a real secret

// newTestServer wires a server around temp dirs and the direct-supervision
// process manager, returning the server and its httptest wrapper.
func newTestServer(t *testing.T) (*server, *httptest.Server) {
	t.Helper()
	root := t.TempDir()
	cfg := config{
		Secret:  testSecret,
		Port:    0,
		AppRoot: filepath.Join(root, "app"),
		LogsDir: filepath.Join(root, "logs"),
		AppPort: 18080,
	}
	if err := ensureDirs(cfg); err != nil {
		t.Fatalf("ensureDirs: %v", err)
	}
	logger := slog.New(slog.NewJSONHandler(io.Discard, nil))
	proc := newDirectManager(cfg, logger)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		proc.Shutdown(ctx)
	})
	srv := newServer(cfg, logger, proc)
	ts := httptest.NewServer(srv.handler())
	t.Cleanup(ts.Close)
	return srv, ts
}

func doJSON(t *testing.T, ts *httptest.Server, method, path, bearer string, body any) (int, map[string]any) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(t.Context(), method, ts.URL+path, reader)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if bearer != "" {
		req.Header.Set("Authorization", "Bearer "+bearer)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	var decoded map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	return resp.StatusCode, decoded
}

// makeBundle builds a real zip bundle whose server binary is a shell script,
// returning the zip bytes and their sha256 hex.
func makeBundle(t *testing.T, script string, extra map[string]string) ([]byte, string) {
	t.Helper()
	entries := []zipEntry{{name: "server", body: script, mode: 0o755}}
	for name, body := range extra {
		entries = append(entries, zipEntry{name: name, body: body})
	}
	path := makeZip(t, entries)
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read bundle: %v", err)
	}
	sum := sha256.Sum256(data)
	return data, hex.EncodeToString(sum[:])
}

// serveBundle exposes bundle bytes over a local httptest server.
func serveBundle(t *testing.T, data []byte) string {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(data)
	}))
	t.Cleanup(ts.Close)
	return ts.URL + "/bundle.zip"
}

// waitFor polls cond until it returns true or the deadline passes.
func waitFor(t *testing.T, timeout time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(25 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", msg)
}
