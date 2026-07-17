package main

import (
	"context"
	"net/http"
	"net/http/httptest"
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
