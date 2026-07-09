package proxy

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/middleware"
)

func driveReverseProxy(t *testing.T, upstreamURL string, attrCache *middleware.AttributionCache, jti string) *http.Response {
	t.Helper()
	u, err := url.Parse(upstreamURL)
	if err != nil {
		t.Fatalf("parse upstream url: %v", err)
	}
	rp := &httputil.ReverseProxy{
		Director: func(req *http.Request) {
			req.URL.Scheme = u.Scheme
			req.URL.Host = u.Host
			req.Host = u.Host
			if err := EnsureOpenRouterUsage(req, openRouterEndUser(attrCache, jti)); err != nil {
				t.Errorf("EnsureOpenRouterUsage: %v", err)
			}
		},
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions",
		strings.NewReader(`{"model":"z-ai/glm-5.2","messages":[{"role":"user","content":"hi"}]}`))
	req.Header.Set("Content-Type", "application/json")
	rp.ServeHTTP(rec, req)
	return rec.Result()
}

func TestOpenRouterEndUserInjectedWhenCached(t *testing.T) {
	sessionID := uuid.New()
	attrCache := middleware.NewAttributionCache(10, time.Minute)
	attrCache.Set("jti-live", middleware.Attribution{SessionID: &sessionID})

	var gotUser string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		payload := map[string]json.RawMessage{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		if raw, ok := payload["user"]; ok {
			_ = json.Unmarshal(raw, &gotUser)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	resp := driveReverseProxy(t, upstream.URL, attrCache, "jti-live")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d", resp.StatusCode)
	}
	if gotUser != sessionID.String() {
		t.Fatalf("user field = %q, want %s", gotUser, sessionID)
	}
}

func TestOpenRouterEndUserAbsentWhenNotCached(t *testing.T) {
	attrCache := middleware.NewAttributionCache(10, time.Minute)

	present := true
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		payload := map[string]json.RawMessage{}
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("decode upstream body: %v", err)
		}
		_, present = payload["user"]
		w.WriteHeader(http.StatusOK)
	}))
	defer upstream.Close()

	resp := driveReverseProxy(t, upstream.URL, attrCache, "jti-missing")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("request should still succeed without cache, status = %d", resp.StatusCode)
	}
	if present {
		t.Fatal("user field should be absent when session not cached")
	}
}
