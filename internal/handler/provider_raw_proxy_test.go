package handler_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVercelProxy_ForwardsRawRequestThroughNango(t *testing.T) {
	var capturedProviderConfigKey string
	var capturedConnectionID string
	var capturedPath string
	var capturedBody string

	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedProviderConfigKey = r.Header.Get("Provider-Config-Key")
		capturedConnectionID = r.Header.Get("Connection-Id")
		capturedPath = r.URL.RequestURI()
		body, _ := io.ReadAll(r.Body)
		capturedBody = string(body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"deployments":[{"uid":"dpl_123","readyState":"READY"}]}`))
	})

	harness := newRawProviderProxyHarness(t, "vercel", nangoHandler)

	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/vercel-proxy/"+harness.agentID.String()+"/v6/deployments?limit=1",
		bytes.NewReader([]byte(`{"name":"web"}`)),
	)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if capturedProviderConfigKey != harness.providerConfigKey {
		t.Fatalf("provider config key = %q, want %q", capturedProviderConfigKey, harness.providerConfigKey)
	}
	if capturedConnectionID != harness.nangoConnectionID {
		t.Fatalf("connection id = %q, want %q", capturedConnectionID, harness.nangoConnectionID)
	}
	if capturedPath != "/proxy/v6/deployments?limit=1" {
		t.Fatalf("path = %q, want proxy deployment path", capturedPath)
	}
	if capturedBody != `{"name":"web"}` {
		t.Fatalf("body = %q", capturedBody)
	}
}

func TestSlackProxy_ForwardsSlackWebAPIMethodThroughNango(t *testing.T) {
	var capturedPath string

	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true,"team":"hivy","user":"hivy"}`))
	})
	harness := newRawProviderProxyHarness(t, "slack", nangoHandler)

	req := httptest.NewRequest(http.MethodGet, "/internal/slack-proxy/"+harness.agentID.String()+"/auth.test", nil)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if capturedPath != "/proxy/auth.test" {
		t.Fatalf("path = %q, want Slack Web API method path", capturedPath)
	}
}

func TestGlitchTipProxy_ForwardsAPI0RequestThroughNango(t *testing.T) {
	var capturedProviderConfigKey string
	var capturedConnectionID string
	var capturedPath string

	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedProviderConfigKey = r.Header.Get("Provider-Config-Key")
		capturedConnectionID = r.Header.Get("Connection-Id")
		capturedPath = r.URL.RequestURI()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[{"id":1,"slug":"web","name":"Web"}]`))
	})

	harness := newRawProviderProxyHarness(t, "glitchtip", nangoHandler)

	req := httptest.NewRequest(
		http.MethodGet,
		"/internal/glitchtip-proxy/"+harness.agentID.String()+"/api/0/projects/?limit=1",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if capturedProviderConfigKey != harness.providerConfigKey {
		t.Fatalf("provider config key = %q, want %q", capturedProviderConfigKey, harness.providerConfigKey)
	}
	if capturedConnectionID != harness.nangoConnectionID {
		t.Fatalf("connection id = %q, want %q", capturedConnectionID, harness.nangoConnectionID)
	}
	if capturedPath != "/proxy/projects/?limit=1" {
		t.Fatalf("path = %q, want GlitchTip API path", capturedPath)
	}
}

func TestGlitchTipProxy_RejectsNonAPI0Path(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called for invalid glitchtip path")
	})
	harness := newRawProviderProxyHarness(t, "glitchtip", nangoHandler)

	req := httptest.NewRequest(http.MethodGet, "/internal/glitchtip-proxy/"+harness.agentID.String()+"/api/settings/", nil)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSlackProxy_RejectsNonSlackWebAPIPath(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called for invalid slack path")
	})
	harness := newRawProviderProxyHarness(t, "slack", nangoHandler)

	req := httptest.NewRequest(http.MethodGet, "/internal/slack-proxy/"+harness.agentID.String()+"/oauth/token", nil)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestSlackProxy_RejectsAPIPrefixPath(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called for invalid slack path")
	})
	harness := newRawProviderProxyHarness(t, "slack", nangoHandler)

	req := httptest.NewRequest(http.MethodGet, "/internal/slack-proxy/"+harness.agentID.String()+"/api/auth.test", nil)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}
}

func TestVercelProxy_DeniesDeleteBeforeNango(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called for denied vercel delete")
	})
	harness := newRawProviderProxyHarness(t, "vercel", nangoHandler)

	req := httptest.NewRequest(
		http.MethodDelete,
		"/internal/vercel-proxy/"+harness.agentID.String()+"/v9/projects/prod-app",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "provider proxy safety policy") {
		t.Fatalf("expected safety policy message, got %s", recorder.Body.String())
	}
}

func TestSlackProxy_DeniesDestructiveMethodBeforeNango(t *testing.T) {
	nangoHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("nango should not be called for denied slack delete")
	})
	harness := newRawProviderProxyHarness(t, "slack", nangoHandler)

	req := httptest.NewRequest(
		http.MethodPost,
		"/internal/slack-proxy/"+harness.agentID.String()+"/chat.delete",
		bytes.NewReader([]byte(`{"channel":"C123","ts":"123.456"}`)),
	)
	req.Header.Set("Authorization", "Bearer "+harness.runtimeSecret)
	req.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	harness.router.ServeHTTP(recorder, req)

	if recorder.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", recorder.Code, recorder.Body.String())
	}
	if !strings.Contains(recorder.Body.String(), "performed by a human directly") {
		t.Fatalf("expected human dashboard guidance, got %s", recorder.Body.String())
	}
}
