package handler_test

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sessionRuntimeStub struct {
	server                         *httptest.Server
	messageStatus                  int
	configCalls                    int
	readyzCalls                    int
	messageCalls                   int
	lastSessionID, lastMessageText string
	lastMessageBody                map[string]any
	lastConfigBody                 map[string]any
	lastSessionContext             []string
	lastAttachments                []any
	lastModelID, lastAPIKeyEnv     string
	lastConfigModelID              string
	lastConfigReasoningEffort      string
}

func newSessionRuntimeStub(t *testing.T, messageStatus int) *sessionRuntimeStub {
	t.Helper()
	runtime := &sessionRuntimeStub{messageStatus: messageStatus}
	runtime.server = httptest.NewServer(http.HandlerFunc(runtime.handle))
	t.Cleanup(runtime.server.Close)
	return runtime
}

func (rt *sessionRuntimeStub) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && r.URL.Path == "/readyz":
		rt.readyzCalls++
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPut && r.URL.Path == "/config":
		rt.handleConfig(w, r)
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/sessions/") && strings.HasSuffix(r.URL.Path, "/messages"):
		rt.handleMessage(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (rt *sessionRuntimeStub) handleConfig(w http.ResponseWriter, r *http.Request) {
	rt.configCalls++
	var body struct {
		Definition *struct {
			Model *struct {
				ModelID         string `json:"model_id"`
				ReasoningEffort string `json:"reasoning_effort"`
			} `json:"model"`
		} `json:"definition"`
	}
	rawBody, err := readSessionRuntimeBody(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	rt.lastConfigBody = map[string]any{}
	_ = json.Unmarshal(rawBody, &rt.lastConfigBody)
	_ = json.Unmarshal(rawBody, &body)
	if body.Definition != nil && body.Definition.Model != nil {
		rt.lastConfigModelID = body.Definition.Model.ModelID
		rt.lastConfigReasoningEffort = body.Definition.Model.ReasoningEffort
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"env_key_count": 1})
}

func readSessionRuntimeBody(r *http.Request) ([]byte, error) {
	rawBody, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(strings.TrimSpace(r.Header.Get("Content-Encoding")), "gzip") {
		return rawBody, nil
	}
	reader, err := gzip.NewReader(bytes.NewReader(rawBody))
	if err != nil {
		return nil, err
	}
	defer reader.Close()
	return io.ReadAll(reader)
}

func (rt *sessionRuntimeStub) handleMessage(w http.ResponseWriter, r *http.Request) {
	rt.messageCalls++
	parts := strings.Split(strings.Trim(r.URL.Path, "/"), "/")
	if len(parts) >= 3 {
		rt.lastSessionID = parts[1]
	}
	var body struct {
		Text            string   `json:"text"`
		SessionContext  []string `json:"session_context"`
		Attachments     []any    `json:"attachments"`
		ModelDefinition *struct {
			ModelID   string `json:"model_id"`
			APIKeyEnv string `json:"api_key_env"`
		} `json:"model_definition"`
	}
	rawBody, _ := io.ReadAll(r.Body)
	_ = json.Unmarshal(rawBody, &body)
	rt.lastMessageBody = map[string]any{}
	_ = json.Unmarshal(rawBody, &rt.lastMessageBody)
	rt.lastMessageText = body.Text
	rt.lastSessionContext = body.SessionContext
	rt.lastAttachments = body.Attachments
	if body.ModelDefinition != nil {
		rt.lastModelID = body.ModelDefinition.ModelID
		rt.lastAPIKeyEnv = body.ModelDefinition.APIKeyEnv
	}
	if rt.messageStatus >= http.StatusBadRequest {
		http.Error(w, "runtime rejected message", rt.messageStatus)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"session_id": rt.lastSessionID,
		"stream_id":  "stream-" + shortTestID(rt.lastSessionID),
		"stream_url": "/sessions/" + rt.lastSessionID + "/stream",
		"trace_id":   "trace-" + shortTestID(rt.lastSessionID),
		"turn_id":    "turn-" + shortTestID(rt.lastSessionID),
	})
}

func shortTestID(value string) string {
	if len(value) < 8 {
		return value
	}
	return value[:8]
}
