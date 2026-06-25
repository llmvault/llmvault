package handler_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type sessionSyncRuntime struct {
	server                         *httptest.Server
	messageStatus                  int
	configCalls                    int
	readyzCalls                    int
	messageCalls                   int
	lastSessionID, lastMessageText string
	lastMessageBody                map[string]any
	lastSessionContext             []string
	lastAttachments                []any
	lastModelID, lastAPIKeyEnv     string
}

func newSessionSyncRuntime(t *testing.T, messageStatus int) *sessionSyncRuntime {
	t.Helper()
	runtime := &sessionSyncRuntime{messageStatus: messageStatus}
	runtime.server = httptest.NewServer(http.HandlerFunc(runtime.handle))
	t.Cleanup(runtime.server.Close)
	return runtime
}

func (rt *sessionSyncRuntime) handle(w http.ResponseWriter, r *http.Request) {
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/healthz":
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet && r.URL.Path == "/readyz":
		rt.readyzCalls++
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodPut && r.URL.Path == "/config":
		rt.configCalls++
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"env_key_count": 1})
	case r.Method == http.MethodPost && strings.HasPrefix(r.URL.Path, "/sessions/") && strings.HasSuffix(r.URL.Path, "/messages"):
		rt.handleMessage(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (rt *sessionSyncRuntime) handleMessage(w http.ResponseWriter, r *http.Request) {
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
