package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVerifyBrowserPreviewCORS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if got := r.Header.Get("Origin"); got != "https://usehivy.com" {
			t.Fatalf("Origin = %q", got)
		}
		if got := r.Header.Get("X-Daytona-Skip-Preview-Warning"); got != "true" {
			t.Fatalf("X-Daytona-Skip-Preview-Warning = %q", got)
		}
		w.Header().Set("Access-Control-Allow-Origin", "https://usehivy.com")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	if err := verifyBrowserPreviewCORS(context.Background(), server.URL); err != nil {
		t.Fatalf("verify browser preview CORS: %v", err)
	}
}

func TestVerifyBrowserPreviewCORSRejectsDuplicateOrigins(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Add("Access-Control-Allow-Origin", "https://usehivy.com")
		w.Header().Add("Access-Control-Allow-Origin", "*")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ok"}`))
	}))
	defer server.Close()

	if err := verifyBrowserPreviewCORS(context.Background(), server.URL); err == nil {
		t.Fatal("expected duplicate Access-Control-Allow-Origin values to fail")
	}
}

func TestVerifyBrowserSessionStreamCORS(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/sessions/daytona-acceptance-session/stream" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if r.URL.Query().Get("replay") != "none" {
			t.Fatalf("replay = %q", r.URL.Query().Get("replay"))
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), "Bearer ") {
			t.Fatalf("Authorization = %q", r.Header.Get("Authorization"))
		}
		if got := r.Header.Get("X-Daytona-Skip-Preview-Warning"); got != "true" {
			t.Fatalf("X-Daytona-Skip-Preview-Warning = %q", got)
		}
		w.Header().Set("Access-Control-Allow-Origin", "https://usehivy.com")
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	if err := verifyBrowserSessionStreamCORS(context.Background(), server.URL, "runtime-secret"); err != nil {
		t.Fatalf("verify browser session stream CORS: %v", err)
	}
}
