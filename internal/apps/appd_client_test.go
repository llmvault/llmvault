package apps

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestAppdClientDeployCallShape(t *testing.T) {
	var got struct {
		method, path, bearer, contentType string
		body                              AppdDeployRequest
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got.method, got.path = r.Method, r.URL.Path
		got.bearer = bearerToken(r)
		got.contentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&got.body)
		_ = json.NewEncoder(w).Encode(AppdDeployResponse{OldSHA: "old", NewSHA: "new"})
	}))
	defer server.Close()

	client := newAppdClient()
	resp, err := client.deploy(context.Background(), server.URL, "hvapp_secret", AppdDeployRequest{
		BundleURL: "https://s3.test/bundle.zip",
		SHA256:    "abc",
		VersionID: "v-1",
		Env:       map[string]string{"HIVY_APP_ID": "id"},
	})
	if err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if got.method != http.MethodPost || got.path != "/deploy" {
		t.Fatalf("call = %s %s", got.method, got.path)
	}
	if got.bearer != "hvapp_secret" {
		t.Fatalf("bearer = %q", got.bearer)
	}
	if got.contentType != "application/json" {
		t.Fatalf("content type = %q", got.contentType)
	}
	if got.body.BundleURL != "https://s3.test/bundle.zip" || got.body.SHA256 != "abc" || got.body.VersionID != "v-1" {
		t.Fatalf("body = %+v", got.body)
	}
	if got.body.Env["HIVY_APP_ID"] != "id" {
		t.Fatalf("body env = %v", got.body.Env)
	}
	if resp.NewSHA != "new" {
		t.Fatalf("resp = %+v", resp)
	}
}

func TestAppdClientEnvPushAndErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/env":
			var req struct {
				Vars map[string]string `json:"vars"`
			}
			_ = json.NewDecoder(r.Body).Decode(&req)
			if req.Vars["KEY"] != "value" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad vars"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"updated": 1})
		case "/logs":
			if r.URL.Query().Get("lines") != "50" || r.URL.Query().Get("grep") != "panic" || r.URL.Query().Get("stream") != "appd" {
				w.WriteHeader(http.StatusBadRequest)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad query"})
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"lines": []string{"a"}, "count": 1})
		default:
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]string{"error": "invalid app secret"})
		}
	}))
	defer server.Close()

	client := newAppdClient()
	ctx := context.Background()
	if err := client.pushEnv(ctx, server.URL, "secret", map[string]string{"KEY": "value"}); err != nil {
		t.Fatalf("pushEnv: %v", err)
	}

	logs, err := client.logs(ctx, server.URL, "secret", LogsQuery{Lines: 50, Grep: "panic", Stream: "appd"})
	if err != nil {
		t.Fatalf("logs: %v", err)
	}
	var decoded struct {
		Count int `json:"count"`
	}
	if err := json.Unmarshal(logs, &decoded); err != nil || decoded.Count != 1 {
		t.Fatalf("logs payload = %s (%v)", logs, err)
	}

	_, err = client.rollback(ctx, server.URL, "secret", "deadbeef")
	var appdErr *AppdError
	if !errors.As(err, &appdErr) || appdErr.StatusCode != http.StatusUnauthorized || appdErr.Message != "invalid app secret" {
		t.Fatalf("rollback error = %v, want AppdError 401", err)
	}
	if isConnectionError(err) {
		t.Fatal("an HTTP-level appd error must not be treated as retryable")
	}
}

// TestDeployWithBootRetry proves connection-refused is retried within the
// window: the target port has no listener at first, then "boots".
func TestDeployWithBootRetry(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := listener.Addr().String()
	_ = listener.Close() // connection refused until the server "boots"

	var served atomic.Bool
	go func() {
		time.Sleep(300 * time.Millisecond)
		late, err := net.Listen("tcp", addr)
		if err != nil {
			return
		}
		served.Store(true)
		server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_ = json.NewEncoder(w).Encode(AppdDeployResponse{NewSHA: "booted"})
		})}
		_ = server.Serve(late)
	}()

	client := newAppdClient()
	client.bootRetryWindow = 5 * time.Second
	client.bootRetryInterval = 50 * time.Millisecond
	resp, err := client.deployWithBootRetry(context.Background(), "http://"+addr, "secret", AppdDeployRequest{
		BundleURL: "https://s3.test/b.zip", SHA256: "abc", VersionID: "v",
	})
	if err != nil {
		t.Fatalf("deployWithBootRetry: %v (server started: %v)", err, served.Load())
	}
	if resp.NewSHA != "booted" {
		t.Fatalf("resp = %+v", resp)
	}
}

// TestDeployWithBootRetryGatewayStatus proves the gateway's 502/503 (appd not
// yet listening) is retried within the boot window and eventually succeeds.
func TestDeployWithBootRetryGatewayStatus(t *testing.T) {
	for _, bootStatus := range []int{http.StatusBadGateway, http.StatusServiceUnavailable} {
		bootStatus := bootStatus
		t.Run(http.StatusText(bootStatus), func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) <= 2 {
					w.WriteHeader(bootStatus)
					_, _ = w.Write([]byte("<html>gateway</html>"))
					return
				}
				_ = json.NewEncoder(w).Encode(AppdDeployResponse{NewSHA: "booted"})
			}))
			defer server.Close()

			client := newAppdClient()
			client.bootRetryWindow = 5 * time.Second
			client.bootRetryInterval = 20 * time.Millisecond
			resp, err := client.deployWithBootRetry(context.Background(), server.URL, "secret", AppdDeployRequest{
				BundleURL: "https://s3.test/b.zip", SHA256: "abc", VersionID: "v",
			})
			if err != nil {
				t.Fatalf("deployWithBootRetry: %v", err)
			}
			if resp.NewSHA != "booted" {
				t.Fatalf("resp = %+v", resp)
			}
			if got := calls.Load(); got != 3 {
				t.Fatalf("appd calls = %d, want 3 (two boot %d then success)", got, bootStatus)
			}
		})
	}
}

// TestDeployWithBootRetryStopsAfterWindow proves a 502 that outlasts the boot
// window fails (non-retryable once the deadline passes), and that a genuine
// non-boot HTTP error (400) is never retried.
func TestDeployWithBootRetryStopsAfterWindow(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte("still down"))
	}))
	defer server.Close()

	client := newAppdClient()
	// Zero window: the deadline is already in the past on the first failure, so
	// the 502 surfaces without a retry (post-window behaviour).
	client.bootRetryWindow = 0
	client.bootRetryInterval = time.Millisecond
	_, err := client.deployWithBootRetry(context.Background(), server.URL, "secret", AppdDeployRequest{
		BundleURL: "https://s3.test/b.zip", SHA256: "abc", VersionID: "v",
	})
	var appdErr *AppdError
	if !errors.As(err, &appdErr) || appdErr.StatusCode != http.StatusBadGateway {
		t.Fatalf("err = %v, want AppdError 502 after window", err)
	}
	if got := calls.Load(); got != 1 {
		t.Fatalf("appd calls = %d, want 1 (no retry past the window)", got)
	}

	// A genuine 400 is never boot-retryable, even with a generous window.
	badReqCalls := atomic.Int32{}
	badServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		badReqCalls.Add(1)
		w.WriteHeader(http.StatusBadRequest)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "bad bundle"})
	}))
	defer badServer.Close()

	client = newAppdClient()
	client.bootRetryWindow = 5 * time.Second
	client.bootRetryInterval = time.Millisecond
	_, err = client.deployWithBootRetry(context.Background(), badServer.URL, "secret", AppdDeployRequest{
		BundleURL: "https://s3.test/b.zip", SHA256: "abc", VersionID: "v",
	})
	if !errors.As(err, &appdErr) || appdErr.StatusCode != http.StatusBadRequest {
		t.Fatalf("err = %v, want AppdError 400 (non-retryable)", err)
	}
	if got := badReqCalls.Load(); got != 1 {
		t.Fatalf("appd calls = %d, want 1 (400 not retried)", got)
	}
}
