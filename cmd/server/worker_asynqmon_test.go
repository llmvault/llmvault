package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/config"
)

// the asynqmon dashboard exposes every queued/archived task payload. It
// must be fail-closed: disabled by default, never mounted without basic-auth
// credentials, and never on the public health port.
func TestBuildAsynqmonServer_FailClosed(t *testing.T) {
	redisOpt := asynq.RedisClientOpt{Addr: "localhost:6379"}
	ctx := context.Background()

	t.Run("disabled by default", func(t *testing.T) {
		cfg := &config.Config{AsynqmonEnabled: false}
		if srv := buildAsynqmonServer(ctx, cfg, redisOpt); srv != nil {
			t.Fatal("dashboard must be nil when disabled")
		}
	})

	t.Run("enabled without credentials refuses", func(t *testing.T) {
		cfg := &config.Config{AsynqmonEnabled: true, AsynqmonPort: 8091, WorkerHealthPort: 8090}
		if srv := buildAsynqmonServer(ctx, cfg, redisOpt); srv != nil {
			t.Fatal("dashboard must be nil when enabled without basic-auth credentials")
		}
	})

	t.Run("refuses to collide with health port", func(t *testing.T) {
		cfg := &config.Config{
			AsynqmonEnabled:  true,
			AsynqmonPort:     8090,
			WorkerHealthPort: 8090,
			AsynqmonUser:     "ops",
			AsynqmonPassword: "secret",
		}
		if srv := buildAsynqmonServer(ctx, cfg, redisOpt); srv != nil {
			t.Fatal("dashboard must not mount on the public health port")
		}
	})

	t.Run("enabled with credentials and dedicated port", func(t *testing.T) {
		cfg := &config.Config{
			AsynqmonEnabled:  true,
			AsynqmonPort:     8091,
			WorkerHealthPort: 8090,
			AsynqmonUser:     "ops",
			AsynqmonPassword: "secret",
		}
		srv := buildAsynqmonServer(ctx, cfg, redisOpt)
		if srv == nil {
			t.Fatal("dashboard should be built when enabled with credentials on a dedicated port")
		}
		if srv.Addr != ":8091" {
			t.Fatalf("dashboard addr = %q, want :8091", srv.Addr)
		}
	})
}

// basicAuth must reject missing/incorrect credentials and allow correct ones.
func TestBasicAuth(t *testing.T) {
	called := false
	h := basicAuth("ops", "secret", http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusOK)
	}))

	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest("GET", "/asynq/", nil))
	if rr.Code != http.StatusUnauthorized || called {
		t.Fatalf("expected 401 without credentials, got %d called=%v", rr.Code, called)
	}
	if rr.Header().Get("WWW-Authenticate") == "" {
		t.Fatal("expected WWW-Authenticate challenge header")
	}

	rr = httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/asynq/", nil)
	req.SetBasicAuth("ops", "wrong")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized || called {
		t.Fatalf("expected 401 with wrong password, got %d called=%v", rr.Code, called)
	}

	rr = httptest.NewRecorder()
	req = httptest.NewRequest("GET", "/asynq/", nil)
	req.SetBasicAuth("ops", "secret")
	h.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK || !called {
		t.Fatalf("expected 200 with correct credentials, got %d called=%v", rr.Code, called)
	}
}
