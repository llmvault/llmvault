package handler_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/model"
)

func TestMCPServers_LegacySSEConnectionTest(t *testing.T) {
	legacy := newLegacySSEMCPFixture(t)
	client := legacy.server.Client()
	client.Timeout = 3 * time.Second
	h := newMCPHarness(t, client, "http://hivy.test/v1/mcp-servers/oauth/callback")
	fx := h.seed(t)
	created := h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.owner, map[string]any{
		"scope": "org", "name": "Legacy SSE MCP", "url": legacy.server.URL + "/legacy-sse",
		"transport": "sse", "auth_type": "static_bearer", "authorization_policy": "service_required",
		"authorization": map[string]any{"principal_type": "org_service", "bearer_token": "legacy-sse-secret"},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create legacy SSE MCP: %d %s", created.Code, created.Body.String())
	}
	serverID := responseMCPServerID(t, created)
	tested := h.request(t, http.MethodPost, "/mcp-servers/"+serverID.String()+"/test", fx.org, &fx.owner, nil)
	if tested.Code != http.StatusOK || !strings.Contains(tested.Body.String(), `"connected":true`) ||
		!strings.Contains(tested.Body.String(), `"protocol_version":"2024-11-05"`) {
		t.Fatalf("test legacy SSE MCP: %d %s", tested.Code, tested.Body.String())
	}
	legacy.mu.Lock()
	defer legacy.mu.Unlock()
	if !legacy.getAuthenticated || legacy.postAuthenticated != 2 {
		t.Fatalf("legacy SSE auth propagation: get=%v post_count=%d", legacy.getAuthenticated, legacy.postAuthenticated)
	}
	if legacy.initializeProtocol != "2024-11-05" || !legacy.initialized {
		t.Fatalf("legacy SSE handshake: protocol=%q initialized=%v", legacy.initializeProtocol, legacy.initialized)
	}
}

func TestMCPServers_OAuthPKCERefreshClientCredentialsAndConnectionTest(t *testing.T) {
	oauth := newOAuthMCPFixture(t)
	callbackURL := "http://hivy.test/v1/mcp-servers/oauth/callback"
	h := newMCPHarness(t, oauth.server.Client(), callbackURL)
	fx := h.seed(t)
	created := h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.owner, map[string]any{
		"scope": "org", "name": "OAuth MCP", "url": oauth.server.URL + "/mcp",
		"auth_type": "oauth_authorization_code", "authorization_policy": "user_required",
		"authorization": map[string]any{"principal_type": "org_service", "client_id": "mcp-client", "client_secret": "client-secret", "scopes": []string{"tools.read"}},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create OAuth MCP: %d %s", created.Code, created.Body.String())
	}
	if !strings.Contains(created.Body.String(), `"status":"pending"`) {
		t.Fatalf("registration-only OAuth client must remain pending: %s", created.Body.String())
	}
	serverID := responseMCPServerID(t, created)
	started := h.request(t, http.MethodPost, "/mcp-servers/"+serverID.String()+"/oauth/start", fx.org, &fx.member, map[string]any{
		"principal_type": "user", "redirect_after": "/w/settings/mcp",
	})
	if started.Code != http.StatusOK {
		t.Fatalf("start OAuth: %d %s", started.Code, started.Body.String())
	}
	var start struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &start); err != nil {
		t.Fatalf("decode OAuth start: %v", err)
	}
	authorizationURL, err := url.Parse(start.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse authorization URL: %v", err)
	}
	if authorizationURL.Path != "/authorize" || authorizationURL.Query().Get("client_id") != "mcp-client" || authorizationURL.Query().Get("code_challenge_method") != "S256" {
		t.Fatalf("authorization URL = %s", start.AuthorizationURL)
	}
	if authorizationURL.Query().Get("resource") != oauth.server.URL+"/mcp" {
		t.Fatalf("resource indicator = %q", authorizationURL.Query().Get("resource"))
	}
	state := authorizationURL.Query().Get("state")
	callback := h.request(t, http.MethodGet, "/v1/mcp-servers/oauth/callback?state="+url.QueryEscape(state)+"&code=test-code", fx.org, nil, nil)
	if callback.Code != http.StatusOK {
		t.Fatalf("OAuth callback: %d %s", callback.Code, callback.Body.String())
	}
	connected := h.request(t, http.MethodGet, "/mcp-servers/"+serverID.String(), fx.org, &fx.member, nil)
	if connected.Code != http.StatusOK || !strings.Contains(connected.Body.String(), `"status":"active"`) {
		t.Fatalf("completed user OAuth is not active: %d %s", connected.Code, connected.Body.String())
	}
	oauth.mu.Lock()
	if oauth.lastCodeVerifier == "" || oauth.lastResource != oauth.server.URL+"/mcp" {
		t.Fatalf("token exchange verifier=%q resource=%q", oauth.lastCodeVerifier, oauth.lastResource)
	}
	oauth.mu.Unlock()
	replayed := h.request(t, http.MethodGet, "/v1/mcp-servers/oauth/callback?state="+url.QueryEscape(state)+"&code=test-code", fx.org, nil, nil)
	if replayed.Code != http.StatusBadRequest {
		t.Fatalf("replayed callback = %d, want 400", replayed.Code)
	}

	tested := h.request(t, http.MethodPost, "/mcp-servers/"+serverID.String()+"/test", fx.org, &fx.member, nil)
	if tested.Code != http.StatusOK || !strings.Contains(tested.Body.String(), `"connected":true`) {
		t.Fatalf("test OAuth MCP: %d %s", tested.Code, tested.Body.String())
	}
	var userAuthorization model.MCPAuthorization
	if err := h.db.WithContext(t.Context()).Where("mcp_server_id = ? AND principal_type = ? AND principal_user_id = ?", serverID, model.MCPPrincipalUser, fx.member.ID).First(&userAuthorization).Error; err != nil {
		t.Fatalf("load user auth: %v", err)
	}
	past := time.Now().Add(-time.Minute)
	if err := h.db.WithContext(t.Context()).Model(&model.MCPAuthorization{}).Where("id = ?", userAuthorization.ID).Update("expires_at", past).Error; err != nil {
		t.Fatalf("expire user auth: %v", err)
	}
	oauth.mu.Lock()
	oauth.expectedToken = "access-refreshed"
	oauth.mu.Unlock()
	tested = h.request(t, http.MethodPost, "/mcp-servers/"+serverID.String()+"/test", fx.org, &fx.member, nil)
	if tested.Code != http.StatusOK {
		t.Fatalf("test refreshed OAuth MCP: %d %s", tested.Code, tested.Body.String())
	}
	oauth.mu.Lock()
	if len(oauth.tokenGrants) < 2 || oauth.tokenGrants[1] != "refresh_token" {
		t.Fatalf("OAuth grants = %#v", oauth.tokenGrants)
	}
	oauth.mu.Unlock()

	clientCredentials := h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.owner, map[string]any{
		"scope": "org", "name": "Machine MCP", "url": oauth.server.URL + "/mcp",
		"auth_type": "oauth_client_credentials", "authorization_policy": "service_required",
		"oauth_metadata": map[string]any{"token_endpoint": oauth.server.URL + "/token"},
		"authorization":  map[string]any{"principal_type": "org_service", "client_id": "mcp-client", "client_secret": "client-secret", "scopes": []string{"tools.read"}},
	})
	if clientCredentials.Code != http.StatusCreated {
		t.Fatalf("create client credentials MCP: %d %s", clientCredentials.Code, clientCredentials.Body.String())
	}
	machineID := responseMCPServerID(t, clientCredentials)
	tested = h.request(t, http.MethodPost, "/mcp-servers/"+machineID.String()+"/test", fx.org, &fx.owner, nil)
	if tested.Code != http.StatusOK {
		t.Fatalf("test client credentials MCP: %d %s", tested.Code, tested.Body.String())
	}
	oauth.mu.Lock()
	grants := append([]string{}, oauth.tokenGrants...)
	oauth.mu.Unlock()
	if grants[len(grants)-1] != "client_credentials" {
		t.Fatalf("client credentials grant missing: %#v", grants)
	}

	for _, testCase := range []struct {
		name          string
		path          string
		authType      string
		policy        string
		headerName    string
		authorization map[string]any
	}{
		{name: "Open Local MCP", path: "/open", authType: "none", policy: "none"},
		{name: "Bearer Local MCP", path: "/static-bearer", authType: "static_bearer", policy: "service_required", authorization: map[string]any{"principal_type": "org_service", "bearer_token": "static-local"}},
		{name: "Header Local MCP", path: "/static-header", authType: "static_header", policy: "service_required", headerName: "X-Local-Key", authorization: map[string]any{"principal_type": "org_service", "header_value": "header-local"}},
	} {
		body := map[string]any{"scope": "org", "name": testCase.name, "url": oauth.server.URL + testCase.path, "auth_type": testCase.authType, "authorization_policy": testCase.policy}
		if testCase.headerName != "" {
			body["header_name"] = testCase.headerName
		}
		if testCase.authorization != nil {
			body["authorization"] = testCase.authorization
		}
		createdLocal := h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.owner, body)
		if createdLocal.Code != http.StatusCreated {
			t.Fatalf("create %s: %d %s", testCase.authType, createdLocal.Code, createdLocal.Body.String())
		}
		localID := responseMCPServerID(t, createdLocal)
		testedLocal := h.request(t, http.MethodPost, "/mcp-servers/"+localID.String()+"/test", fx.org, &fx.owner, nil)
		if testedLocal.Code != http.StatusOK {
			t.Fatalf("test %s: %d %s", testCase.authType, testedLocal.Code, testedLocal.Body.String())
		}
	}

	var rows []model.MCPAuthorization
	if err := h.db.WithContext(t.Context()).Where("org_id = ?", fx.org.ID).Find(&rows).Error; err != nil {
		t.Fatalf("load OAuth rows: %v", err)
	}
	for _, row := range rows {
		ciphertext := string(row.CredentialsEncrypted)
		for _, secret := range []string{"client-secret", "access-initial", "refresh-initial", "access-refreshed", "refresh-rotated", "access-client-credentials"} {
			if strings.Contains(ciphertext, secret) {
				t.Fatalf("OAuth secret %q stored in plaintext", secret)
			}
		}
	}
}

func TestMCPServers_OAuthDynamicClientRegistration(t *testing.T) {
	oauth := newOAuthMCPFixture(t)
	h := newMCPHarness(t, oauth.server.Client(), "http://hivy.test/v1/mcp-servers/oauth/callback")
	fx := h.seed(t)
	created := h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.owner, map[string]any{
		"scope": "org", "name": "DCR MCP", "url": oauth.server.URL + "/dcr-mcp",
		"auth_type": "oauth_authorization_code", "authorization_policy": "user_required",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create DCR MCP: %d %s", created.Code, created.Body.String())
	}
	serverID := responseMCPServerID(t, created)
	started := h.request(t, http.MethodPost, "/mcp-servers/"+serverID.String()+"/oauth/start", fx.org, &fx.member, map[string]any{"principal_type": "user", "scopes": []string{"tools.read"}})
	if started.Code != http.StatusOK {
		t.Fatalf("start DCR OAuth: %d %s", started.Code, started.Body.String())
	}
	var response struct {
		AuthorizationURL string `json:"authorization_url"`
	}
	if err := json.Unmarshal(started.Body.Bytes(), &response); err != nil {
		t.Fatalf("decode DCR start: %v", err)
	}
	redirect, err := url.Parse(response.AuthorizationURL)
	if err != nil {
		t.Fatalf("parse DCR authorization URL: %v", err)
	}
	if redirect.Query().Get("client_id") != "mcp-client" {
		t.Fatalf("DCR client id = %q", redirect.Query().Get("client_id"))
	}
	oauth.mu.Lock()
	registrations := oauth.registrations
	oauth.mu.Unlock()
	if registrations != 1 {
		t.Fatalf("DCR registrations = %d, want 1", registrations)
	}
	callback := h.request(t, http.MethodGet, "/v1/mcp-servers/oauth/callback?state="+url.QueryEscape(redirect.Query().Get("state"))+"&code=dcr-code", fx.org, nil, nil)
	if callback.Code != http.StatusOK {
		t.Fatalf("DCR callback: %d %s", callback.Code, callback.Body.String())
	}
	tested := h.request(t, http.MethodPost, "/mcp-servers/"+serverID.String()+"/test", fx.org, &fx.member, nil)
	if tested.Code != http.StatusOK {
		t.Fatalf("test DCR MCP: %d %s", tested.Code, tested.Body.String())
	}
}
