package handler_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newAgentSidecarServer(t *testing.T, stub *sidecarStub) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/gateway/http/streams/") {
			handleSidecarHTTPStream(w, r, stub)
			return
		}
		switch r.URL.Path {
		case "/healthz":
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(`ok`))
		case "/readyz":
			if r.Header.Get("Authorization") == "" {
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			w.WriteHeader(http.StatusOK)
		case "/config":
			handleSidecarConfig(w, r, stub)
		case "/config/env":
			handleSidecarEnv(w, r, stub)
		case "/gateway/http/messages":
			handleSidecarHTTPMessage(w, r, stub)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func handleSidecarHTTPStream(w http.ResponseWriter, r *http.Request, stub *sidecarStub) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	stub.mu.Lock()
	body := stub.httpStreamBody
	stub.mu.Unlock()
	if body == "" {
		body = "event: token\ndata: {\"text\":\"hello\"}\n\nevent: done\ndata: {\"session_id\":\"http-web\"}\n\n"
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(body))
}

func handleSidecarConfig(w http.ResponseWriter, r *http.Request, stub *sidecarStub) {
	if r.Method == http.MethodGet {
		stub.mu.Lock()
		body := append([]byte(nil), stub.lastConfigBody...)
		stub.mu.Unlock()
		if len(body) == 0 {
			body = []byte(`{"agent":{"name":"Hivy"},"system_prompt":{"cacheable_segments":[{"type":"static_text","config":{"content":"test"}}],"dynamic_segments":[]},"model":{"provider":"openai_compatible","base_url":"https://proxy.test/v1","model_id":"test","api_key_env":"HIVY_PROXY_API_KEY","temperature":0,"max_output_tokens":1000},"tools":[]}`)
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(body)
		return
	}
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	stub.mu.Lock()
	stub.syncConfigCalls++
	stub.lastSyncBearer = r.Header.Get("Authorization")
	stub.lastConfigBody = body
	stub.lastRawConfigBody = body
	status := stub.syncConfigStatus
	errs := append([]string(nil), stub.syncConfigErrors...)
	var payload struct {
		RuntimeEnv map[string]string `json:"runtime_env"`
		Definition json.RawMessage   `json:"definition"`
	}
	if err := json.Unmarshal(body, &payload); err == nil && len(payload.RuntimeEnv) > 0 {
		stub.syncEnvCalls++
		stub.lastEnvBearer = r.Header.Get("Authorization")
		envBody, _ := json.Marshal(payload.RuntimeEnv)
		stub.lastEnvBody = envBody
		if len(payload.Definition) > 0 {
			stub.lastConfigBody = payload.Definition
		}
	}
	stub.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	respBody := map[string]any{
		"applied_at": "2026-01-01T00:00:00Z", "env_key_count": len(payload.RuntimeEnv), "secret_rotated": false,
	}
	if len(errs) > 0 {
		respBody["errors"] = errs
	}
	_ = json.NewEncoder(w).Encode(respBody)
}

func handleSidecarEnv(w http.ResponseWriter, r *http.Request, stub *sidecarStub) {
	if r.Method != http.MethodPut {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	stub.mu.Lock()
	stub.syncEnvCalls++
	stub.lastEnvBearer = r.Header.Get("Authorization")
	stub.lastEnvBody = body
	status := stub.syncConfigStatus
	stub.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	var env map[string]string
	if err := json.Unmarshal(body, &env); err != nil {
		w.WriteHeader(http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"applied_at": "2026-01-01T00:00:00Z",
		"key_count":  len(env),
	})
}

func handleSidecarHTTPMessage(w http.ResponseWriter, r *http.Request, stub *sidecarStub) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	body, _ := io.ReadAll(r.Body)
	var payload struct {
		ConversationID string `json:"conversation_id"`
	}
	_ = json.Unmarshal(body, &payload)
	if payload.ConversationID == "" {
		payload.ConversationID = "web-default"
	}
	sessionID := payload.ConversationID
	if !strings.HasPrefix(sessionID, "http-") {
		sessionID = "http-" + sessionID
	}
	stub.mu.Lock()
	stub.httpMessageCalls++
	call := stub.httpMessageCalls
	stub.lastHTTPBearer = r.Header.Get("Authorization")
	stub.lastHTTPBody = body
	stub.mu.Unlock()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"session_id":          sessionID,
		"stream_id":           fmt.Sprintf("stream-%d", call),
		"stream_url":          fmt.Sprintf("/gateway/http/streams/stream-%d", call),
		"response_stream_id":  fmt.Sprintf("response-stream-%d", call),
		"response_stream_url": fmt.Sprintf("/gateway/http/response-streams/response-stream-%d", call),
		"trace_id":            fmt.Sprintf("trace-%d", call),
		"turn_id":             fmt.Sprintf("turn-%d", call),
	})
}
