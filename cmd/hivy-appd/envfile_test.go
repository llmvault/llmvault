package main

import (
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"
)

func TestEnvFileRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env")
	vars := map[string]string{
		"PLAIN":      "value",
		"WITH_QUOTE": `say "hi"`,
		"WITH_SLASH": `C:\path\to`,
		"EMPTY":      "",
	}
	if err := writeEnvFile(path, vars); err != nil {
		t.Fatalf("writeEnvFile: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("env file perms = %v, want 0600", info.Mode().Perm())
	}

	got, err := parseEnvFile(path)
	if err != nil {
		t.Fatalf("parseEnvFile: %v", err)
	}
	want := []string{"EMPTY=", "PLAIN=value", `WITH_QUOTE=say "hi"`, `WITH_SLASH=C:\path\to`}
	slices.Sort(got)
	if !slices.Equal(got, want) {
		t.Errorf("parsed = %v, want %v", got, want)
	}
}

func TestWriteEnvFileRejectsBadInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "env")
	if err := writeEnvFile(path, map[string]string{"BAD-NAME": "x"}); err == nil {
		t.Error("expected invalid name rejection")
	}
	if err := writeEnvFile(path, map[string]string{"1LEADING": "x"}); err == nil {
		t.Error("expected leading-digit rejection")
	}
	if err := writeEnvFile(path, map[string]string{"OK": "line1\nline2"}); err == nil {
		t.Error("expected newline rejection")
	}
	if _, err := os.Stat(path); err == nil {
		t.Error("env file must not be created on validation failure")
	}
}

func TestParseEnvFileMissingIsEmpty(t *testing.T) {
	vars, err := parseEnvFile(filepath.Join(t.TempDir(), "nonexistent"))
	if err != nil || vars != nil {
		t.Fatalf("got %v, %v; want nil, nil", vars, err)
	}
}

func TestEnvEndpointRewritesAndRestarts(t *testing.T) {
	srv, ts := newTestServer(t)

	// Deploy first so the restart actually cycles a real process.
	bundle, sha := makeBundle(t, longRunningScript, nil)
	status, _ := doJSON(t, ts, http.MethodPost, "/deploy", testSecret, map[string]any{
		"bundle_url": serveBundle(t, bundle),
		"sha256":     sha,
	})
	if status != http.StatusOK {
		t.Fatalf("deploy status = %d", status)
	}

	status, body := doJSON(t, ts, http.MethodPost, "/env", testSecret, map[string]any{
		"vars": map[string]string{"GREETING": "updated-greeting"},
	})
	if status != http.StatusOK {
		t.Fatalf("env status = %d, body = %v", status, body)
	}
	if body["restarted"] != true {
		t.Errorf("body = %v, want restarted=true", body)
	}
	waitFor(t, 5*time.Second, func() bool {
		data, err := os.ReadFile(srv.cfg.appLogPath())
		return err == nil && strings.Contains(string(data), "GREETING=updated-greeting")
	}, "app to restart with the new env")

	status, _ = doJSON(t, ts, http.MethodPost, "/env", testSecret, map[string]any{
		"vars": map[string]string{"BAD NAME": "x"},
	})
	if status != http.StatusBadRequest {
		t.Errorf("invalid var name status = %d, want 400", status)
	}
}
