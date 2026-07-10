package e2e

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
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
	mu     sync.Mutex
	bodies []map[string]any
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
	p.recordBody(body)
	p.trace.Body("model-proxy", "upstream OpenRouter request", body)
	upstream, err := http.NewRequestWithContext(r.Context(), http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	upstream.Header.Set("Authorization", "Bearer "+p.key)
	upstream.Header.Set("Content-Type", r.Header.Get("Content-Type"))
	upstream.Header.Set("HTTP-Referer", "https://usehivy.test")
	upstream.Header.Set("X-Title", "Hivy")
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

func (p *agentRuntimeModelProxy) assertModelPayloads(t *testing.T, modelProfile string) {
	t.Helper()
	bodies := p.capturedBodies()
	if len(bodies) == 0 {
		t.Fatal("no model proxy payloads captured")
	}
	for i, body := range bodies {
		if modelProfile != "openrouter_compatible" {
			if _, ok := body["reasoning_effort"]; ok {
				t.Fatalf("payload %d forwarded generic reasoning_effort: %#v", i, body["reasoning_effort"])
			}
			if _, ok := body["parallel_tool_calls"]; ok {
				t.Fatalf("payload %d forwarded parallel_tool_calls for %s profile: %#v", i, modelProfile, body["parallel_tool_calls"])
			}
		}
		assertNoUnsupportedToolSchemaKeywords(t, body, i)
	}
}

func assertNoUnsupportedToolSchemaKeywords(t *testing.T, value any, payloadIndex int) {
	t.Helper()
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			switch key {
			case "$defs", "definitions", "$schema", "$id":
				t.Fatalf("payload %d contains unsupported schema keyword %q", payloadIndex, key)
			}
			assertNoUnsupportedToolSchemaKeywords(t, child, payloadIndex)
		}
	case []any:
		for _, child := range typed {
			assertNoUnsupportedToolSchemaKeywords(t, child, payloadIndex)
		}
	}
}

func (p *agentRuntimeModelProxy) recordBody(body []byte) {
	var payload map[string]any
	if err := json.Unmarshal(body, &payload); err != nil {
		payload = map[string]any{"_decode_error": err.Error()}
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.bodies = append(p.bodies, payload)
}

func (p *agentRuntimeModelProxy) capturedBodies() []map[string]any {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := make([]map[string]any, len(p.bodies))
	copy(out, p.bodies)
	return out
}
