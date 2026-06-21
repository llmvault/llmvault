package runner

import (
	"context"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestHTTPHealthCheckWaitsForDelayedSuccess(t *testing.T) {
	var attempts int32
	server := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if atomic.AddInt32(&attempts, 1) < 3 {
			http.Error(w, "not ready", http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
	})}
	listener := listenLocalhost(t)
	defer func() { _ = server.Close() }()
	go func() { _ = server.Serve(listener) }()

	err := waitForHTTPHealthCheck(context.Background(), "sbx_health", 7080, listenerPort(t, listener), HealthCheckConfig{
		Type:           "http",
		Method:         "GET",
		Path:           "/healthz",
		ExpectedStatus: http.StatusOK,
		TimeoutSeconds: 2,
		IntervalMS:     10,
	})
	if err != nil {
		t.Fatalf("waitForHTTPHealthCheck: %v", err)
	}
	if got := atomic.LoadInt32(&attempts); got < 3 {
		t.Fatalf("attempts = %d, want at least 3", got)
	}
}

func TestHTTPHealthCheckTimeoutIncludesLastStatus(t *testing.T) {
	server := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	})}
	listener := listenLocalhost(t)
	defer func() { _ = server.Close() }()
	go func() { _ = server.Serve(listener) }()

	err := waitForHTTPHealthCheck(context.Background(), "sbx_health", 7080, listenerPort(t, listener), HealthCheckConfig{
		Type:           "http",
		Method:         "GET",
		Path:           "/healthz",
		ExpectedStatus: http.StatusOK,
		TimeoutSeconds: 1,
		IntervalMS:     10,
	})
	if err == nil {
		t.Fatal("expected health check timeout")
	}
	if got := err.Error(); !strings.Contains(got, "status=503") {
		t.Fatalf("error = %q, want last status", got)
	}
}

func TestHTTPHealthCheckContextDeadlineIncludesLastStatus(t *testing.T) {
	server := http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "not ready", http.StatusServiceUnavailable)
	})}
	listener := listenLocalhost(t)
	defer func() { _ = server.Close() }()
	go func() { _ = server.Serve(listener) }()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := waitForHTTPHealthCheck(ctx, "sbx_health", 7080, listenerPort(t, listener), HealthCheckConfig{
		Type:           "http",
		Method:         "GET",
		Path:           "/healthz",
		ExpectedStatus: http.StatusOK,
		TimeoutSeconds: 30,
		IntervalMS:     10,
	})
	if err == nil {
		t.Fatal("expected health check context timeout")
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("health check elapsed %s, want context deadline to bound it", elapsed)
	}
	if got := err.Error(); !strings.Contains(got, "status=503") {
		t.Fatalf("error = %q, want last status", got)
	}
}

func listenLocalhost(t *testing.T) net.Listener {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	return listener
}

func listenerPort(t *testing.T, listener net.Listener) int {
	t.Helper()
	_, rawPort, err := net.SplitHostPort(listener.Addr().String())
	if err != nil {
		t.Fatalf("split listener addr: %v", err)
	}
	port, err := strconv.Atoi(rawPort)
	if err != nil {
		t.Fatalf("parse listener port: %v", err)
	}
	return port
}
