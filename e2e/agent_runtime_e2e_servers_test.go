package e2e

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

type agentRuntimeModelProxy struct {
	server *httptest.Server
	trace  *agentRuntimeE2ETrace
	token  string
	key    string
	calls  atomic.Int64
}

func newAgentRuntimeModelProxy(t *testing.T, trace *agentRuntimeE2ETrace, systemModelKey, proxyToken string) *agentRuntimeModelProxy {
	t.Helper()
	p := &agentRuntimeModelProxy{trace: trace, token: proxyToken, key: systemModelKey}
	p.server = newPublicHTTPServer(t, http.HandlerFunc(p.serveHTTP))
	trace.Logf("model-proxy", "server listening url=%s container_url=%s", p.server.URL, containerURLForServer(p.server.URL))
	return p
}

func (p *agentRuntimeModelProxy) serveHTTP(w http.ResponseWriter, r *http.Request) {
	started := time.Now()
	p.trace.Logf("model-proxy", "incoming method=%s path=%s", r.Method, r.URL.Path)
	if r.Header.Get("Authorization") != "Bearer "+p.token {
		p.trace.Logf("model-proxy", "rejecting request with invalid bearer token")
		http.Error(w, "bad proxy token", http.StatusUnauthorized)
		return
	}
	if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/chat/completions") {
		p.trace.Logf("model-proxy", "rejecting unsupported request path=%s", r.URL.Path)
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	p.calls.Add(1)
	body, _ := io.ReadAll(r.Body)
	p.trace.Body("model-proxy", "upstream OpenRouter request", body)
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	upstream.Header.Set("Authorization", "Bearer "+p.key)
	upstream.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	upstream.Header.Set("HTTP-Referer", "https://usehivy.com")
	upstream.Header.Set("X-Title", "Hivy agent runtime E2E")
	resp, err := http.DefaultClient.Do(upstream)
	if err != nil {
		p.trace.Logf("model-proxy", "upstream OpenRouter error: %v", err)
		http.Error(w, err.Error(), http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	p.trace.Body("model-proxy", fmt.Sprintf("upstream OpenRouter response status=%d elapsed=%s", resp.StatusCode, time.Since(started).Truncate(time.Millisecond)), respBody)
	for key, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(key, value)
		}
	}
	w.WriteHeader(resp.StatusCode)
	_, _ = w.Write(respBody)
}

func (p *agentRuntimeModelProxy) assertUsed(t *testing.T) {
	t.Helper()
	calls := p.calls.Load()
	if calls == 0 {
		t.Fatalf("model proxy was not called")
	}
	p.trace.Logf("assert", "model proxy calls=%d", calls)
}

type agentRuntimeMockControlPlane struct {
	server        *httptest.Server
	trace         *agentRuntimeE2ETrace
	containerURL  string
	runtimeSecret string
	agentID       string
	sandboxID     string

	mu              sync.Mutex
	presignRequests int
	uploadRequests  int
	confirmRequests int
	webhookRequests int
	batchRequests   int
	uploadSHA256    string
	payloads        []string
	paths           []string
	eventTypes      []string
}

func newAgentRuntimeMockControlPlane(t *testing.T, trace *agentRuntimeE2ETrace, runtimeSecret, agentID, sandboxID string) *agentRuntimeMockControlPlane {
	t.Helper()
	cp := &agentRuntimeMockControlPlane{trace: trace, runtimeSecret: runtimeSecret, agentID: agentID, sandboxID: sandboxID}
	cp.server = newPublicHTTPServer(t, http.HandlerFunc(cp.serveHTTP))
	cp.containerURL = containerURLForServer(cp.server.URL)
	trace.Logf("control-plane", "server listening url=%s container_url=%s", cp.server.URL, cp.containerURL)
	return cp
}

func (cp *agentRuntimeMockControlPlane) serveHTTP(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	path := r.URL.Path
	cp.trace.Logf("control-plane", "incoming method=%s path=%s bytes=%d", r.Method, path, len(body))
	switch {
	case r.Method == http.MethodPost && path == "/internal/agents/"+cp.agentID+"/sqlite-backup/presign":
		if r.Header.Get("Authorization") != "Bearer "+cp.runtimeSecret {
			http.Error(w, "bad bearer", http.StatusUnauthorized)
			return
		}
		cp.record("presign", path, "", body)
		cp.trace.Body("control-plane", "sqlite backup presign request", body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fmt.Sprintf(`{"key":"agent-runtime-e2e.db.gz","upload_url":"%s/upload/agent-runtime-e2e.db.gz"}`, cp.containerURL)))
	case r.Method == http.MethodPut && path == "/upload/agent-runtime-e2e.db.gz":
		sum := sha256.Sum256(body)
		cp.mu.Lock()
		cp.uploadRequests++
		cp.uploadSHA256 = hex.EncodeToString(sum[:])
		cp.paths = append(cp.paths, path)
		cp.mu.Unlock()
		cp.trace.Logf("control-plane", "sqlite backup uploaded bytes=%d sha256=%s", len(body), hex.EncodeToString(sum[:]))
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPost && path == "/internal/agents/"+cp.agentID+"/sqlite-backup/confirm":
		if r.Header.Get("Authorization") != "Bearer "+cp.runtimeSecret {
			http.Error(w, "bad bearer", http.StatusUnauthorized)
			return
		}
		cp.record("confirm", path, "", body)
		cp.trace.Body("control-plane", "sqlite backup confirm request", body)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && path == "/internal/webhooks/agent/"+cp.sandboxID:
		cp.record("webhook", path, r.Header.Get("X-Hivy-Event-Type"), body)
		cp.trace.Body("control-plane", "agent webhook request", body)
		w.WriteHeader(http.StatusNoContent)
	case r.Method == http.MethodPost && path == "/internal/webhooks/agent/"+cp.sandboxID+"/batch":
		if err := cp.recordBatch(path, body); err != nil {
			cp.trace.Logf("control-plane", "bad batch webhook: %v", err)
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusNoContent)
	default:
		cp.trace.Logf("control-plane", "unhandled request method=%s path=%s", r.Method, path)
		http.Error(w, "not found", http.StatusNotFound)
	}
}

func (cp *agentRuntimeMockControlPlane) record(kind, path, eventType string, body []byte) {
	cp.mu.Lock()
	defer cp.mu.Unlock()
	switch kind {
	case "presign":
		cp.presignRequests++
	case "confirm":
		cp.confirmRequests++
	case "webhook":
		cp.webhookRequests++
	}
	cp.paths = append(cp.paths, path)
	if eventType != "" {
		cp.eventTypes = append(cp.eventTypes, eventType)
	}
	if len(body) > 0 {
		cp.payloads = append(cp.payloads, string(body))
	}
}

func (cp *agentRuntimeMockControlPlane) recordBatch(path string, body []byte) error {
	reader, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return err
	}
	defer reader.Close()
	decoded, err := io.ReadAll(reader)
	if err != nil {
		return err
	}
	cp.trace.Body("control-plane", "agent batch webhook request", decoded)
	var eventTypes []string
	for _, line := range bytes.Split(decoded, []byte{'\n'}) {
		line = bytes.TrimSpace(line)
		if len(line) == 0 {
			continue
		}
		var event map[string]any
		if err := json.Unmarshal(line, &event); err != nil {
			cp.trace.Logf("control-plane", "skipping malformed batch event: %v", err)
			continue
		}
		eventType, _ := event["event_type"].(string)
		eventTypes = append(eventTypes, eventType)
		cp.mu.Lock()
		if eventType != "" {
			cp.eventTypes = append(cp.eventTypes, eventType)
		}
		cp.payloads = append(cp.payloads, string(line))
		cp.mu.Unlock()
	}
	cp.mu.Lock()
	cp.batchRequests++
	cp.paths = append(cp.paths, path)
	cp.mu.Unlock()
	cp.trace.Logf("control-plane", "agent batch webhook accepted events=%d event_types=%v", len(eventTypes), eventTypes)
	return nil
}

func newPublicHTTPServer(t *testing.T, handler http.Handler) *httptest.Server {
	t.Helper()
	listener, err := net.Listen("tcp", "0.0.0.0:0") // #nosec G102 -- e2e server must be reachable from Docker.
	if err != nil {
		t.Fatalf("listen public test server: %v", err)
	}
	server := httptest.NewUnstartedServer(handler)
	server.Listener = listener
	server.Start()
	return server
}

func containerURLForServer(rawURL string) string {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return rawURL
	}
	_, port, err := net.SplitHostPort(parsed.Host)
	if err != nil {
		return rawURL
	}
	parsed.Host = net.JoinHostPort("host.docker.internal", port)
	return parsed.String()
}
