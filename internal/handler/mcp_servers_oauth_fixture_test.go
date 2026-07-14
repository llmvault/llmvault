package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
)

const callbackURLPathFragment = "oauth/callback"

type oauthMCPFixture struct {
	server           *httptest.Server
	mu               sync.Mutex
	tokenGrants      []string
	expectedToken    string
	lastCodeVerifier string
	lastResource     string
	registrations    int
}

func newOAuthMCPFixture(t *testing.T) *oauthMCPFixture {
	t.Helper()
	fixture := &oauthMCPFixture{expectedToken: "access-initial"}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/open":
			writeLocalMCPInitialize(w)
		case "/static-bearer":
			if r.Header.Get("Authorization") != "Bearer static-local" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			writeLocalMCPInitialize(w)
		case "/static-header":
			if r.Header.Get("X-Local-Key") != "header-local" {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			writeLocalMCPInitialize(w)
		case "/mcp":
			if r.Method == http.MethodGet {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+fixture.server.URL+`/protected-resource"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			fixture.mu.Lock()
			expected := fixture.expectedToken
			fixture.mu.Unlock()
			if r.Header.Get("Authorization") != "Bearer "+expected {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			writeLocalMCPInitialize(w)
		case "/dcr-mcp":
			if r.Method == http.MethodGet {
				w.Header().Set("WWW-Authenticate", `Bearer resource_metadata="`+fixture.server.URL+`/dcr-protected"`)
				w.WriteHeader(http.StatusUnauthorized)
				return
			}
			fixture.mu.Lock()
			expected := fixture.expectedToken
			fixture.mu.Unlock()
			if r.Header.Get("Authorization") != "Bearer "+expected {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			writeLocalMCPInitialize(w)
		case "/protected-resource":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": fixture.server.URL + "/mcp", "authorization_servers": []string{fixture.server.URL}, "scopes_supported": []string{"tools.read"}})
		case "/dcr-protected":
			_ = json.NewEncoder(w).Encode(map[string]any{"resource": fixture.server.URL + "/dcr-mcp", "authorization_servers": []string{fixture.server.URL + "/dcr-issuer"}, "scopes_supported": []string{"tools.read"}})
		case "/.well-known/oauth-authorization-server":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": fixture.server.URL, "authorization_endpoint": fixture.server.URL + "/authorize", "token_endpoint": fixture.server.URL + "/token", "scopes_supported": []string{"tools.read"}})
		case "/.well-known/oauth-authorization-server/dcr-issuer":
			_ = json.NewEncoder(w).Encode(map[string]any{"issuer": fixture.server.URL + "/dcr-issuer", "authorization_endpoint": fixture.server.URL + "/authorize", "token_endpoint": fixture.server.URL + "/token", "registration_endpoint": fixture.server.URL + "/register", "scopes_supported": []string{"tools.read"}})
		case "/register":
			var registration map[string]any
			if err := json.NewDecoder(r.Body).Decode(&registration); err != nil {
				http.Error(w, "bad registration", http.StatusBadRequest)
				return
			}
			fixture.mu.Lock()
			fixture.registrations++
			fixture.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"client_id": "mcp-client", "client_secret": "client-secret"})
		case "/token":
			if err := r.ParseForm(); err != nil {
				http.Error(w, "bad form", http.StatusBadRequest)
				return
			}
			clientID, clientSecret, _ := r.BasicAuth()
			if clientID != "mcp-client" || clientSecret != "client-secret" {
				http.Error(w, "bad client", http.StatusUnauthorized)
				return
			}
			grant := r.Form.Get("grant_type")
			fixture.mu.Lock()
			fixture.tokenGrants = append(fixture.tokenGrants, grant)
			fixture.lastCodeVerifier = r.Form.Get("code_verifier")
			fixture.lastResource = r.Form.Get("resource")
			token := "access-initial"
			refresh := "refresh-initial"
			if grant == "refresh_token" {
				token = "access-refreshed"
				refresh = "refresh-rotated"
			}
			if grant == "client_credentials" {
				token = "access-client-credentials"
				refresh = ""
			}
			fixture.expectedToken = token
			fixture.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"access_token": token, "refresh_token": refresh, "token_type": "Bearer", "expires_in": 3600, "scope": "tools.read"})
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func writeLocalMCPInitialize(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"jsonrpc": "2.0", "id": 1,
		"result": map[string]any{"protocolVersion": "2025-11-25", "serverInfo": map[string]any{"name": "local-test", "version": "1"}, "capabilities": map[string]any{"tools": map[string]any{}}},
	})
}

type legacySSEMCPFixture struct {
	server *httptest.Server
	input  chan map[string]any

	mu                 sync.Mutex
	getAuthenticated   bool
	postAuthenticated  int
	initializeProtocol string
	initialized        bool
}

func newLegacySSEMCPFixture(t *testing.T) *legacySSEMCPFixture {
	t.Helper()
	fixture := &legacySSEMCPFixture{input: make(chan map[string]any, 1)}
	fixture.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer legacy-sse-secret" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/legacy-sse":
			fixture.mu.Lock()
			fixture.getAuthenticated = true
			fixture.mu.Unlock()
			flusher, ok := w.(http.Flusher)
			if !ok {
				http.Error(w, "streaming unsupported", http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "text/event-stream; charset=utf-8")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("event: endpoint\ndata: /legacy-messages?session_id=local\n\n"))
			flusher.Flush()
			select {
			case message := <-fixture.input:
				params, _ := message["params"].(map[string]any)
				protocolVersion, _ := params["protocolVersion"].(string)
				fixture.mu.Lock()
				fixture.initializeProtocol = protocolVersion
				fixture.mu.Unlock()
				response, _ := json.Marshal(map[string]any{
					"jsonrpc": "2.0", "id": message["id"],
					"result": map[string]any{
						"protocolVersion": "2024-11-05",
						"serverInfo":      map[string]any{"name": "legacy-local", "version": "1"},
						"capabilities":    map[string]any{"tools": map[string]any{}},
					},
				})
				_, _ = w.Write([]byte("event: message\ndata: " + string(response) + "\n\n"))
				flusher.Flush()
				<-r.Context().Done()
			case <-r.Context().Done():
			}
		case r.Method == http.MethodPost && r.URL.Path == "/legacy-messages":
			var message map[string]any
			if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
				http.Error(w, "bad message", http.StatusBadRequest)
				return
			}
			fixture.mu.Lock()
			fixture.postAuthenticated++
			fixture.mu.Unlock()
			switch message["method"] {
			case "initialize":
				fixture.input <- message
			case "notifications/initialized":
				fixture.mu.Lock()
				fixture.initialized = true
				fixture.mu.Unlock()
			default:
				http.Error(w, "unsupported method", http.StatusBadRequest)
				return
			}
			w.WriteHeader(http.StatusAccepted)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(fixture.server.Close)
	return fixture
}
