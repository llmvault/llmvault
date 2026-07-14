package handler_test

import (
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/mcpservers"
	"github.com/usehivy/hivy/internal/model"
)

func TestMCPServers_PersonalOrgAssignmentsAndSecretRedaction(t *testing.T) {
	h := newMCPHarness(t, nil, "http://hivy.test/v1/mcp-servers/oauth/callback")
	fx := h.seed(t)
	orgSecret := "org-secret-" + uuid.NewString()
	created := h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.owner, map[string]any{
		"scope": "org", "name": "Shared Search", "url": "https://mcp.example.test/rpc",
		"auth_type": "static_bearer", "authorization_policy": "service_required",
		"authorization": map[string]any{"principal_type": "org_service", "bearer_token": orgSecret},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create org MCP: code=%d body=%s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), orgSecret) {
		t.Fatal("create response leaked bearer token")
	}
	orgServerID := responseMCPServerID(t, created)

	personalSecret := "personal-secret-" + uuid.NewString()
	created = h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.member, map[string]any{
		"scope": "personal", "name": "My Dev MCP", "url": "https://personal.example.test/mcp",
		"auth_type": "static_header", "header_name": "X-Dev-Key",
		"authorization": map[string]any{"principal_type": "user", "header_value": personalSecret},
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create personal MCP: code=%d body=%s", created.Code, created.Body.String())
	}
	if strings.Contains(created.Body.String(), personalSecret) {
		t.Fatal("create response leaked personal header")
	}
	personalServerID := responseMCPServerID(t, created)
	created = h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.member, map[string]any{
		"scope": "personal", "name": "My Docs MCP", "url": "https://docs.example.test/mcp",
		"auth_type": "none", "authorization_policy": "none",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create second personal MCP: code=%d body=%s", created.Code, created.Body.String())
	}
	secondPersonalServerID := responseMCPServerID(t, created)

	listed := h.request(t, http.MethodGet, "/mcp-servers", fx.org, &fx.owner, nil)
	if listed.Code != http.StatusOK {
		t.Fatalf("owner list: %d %s", listed.Code, listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), personalServerID.String()) {
		t.Fatal("owner saw another user's personal MCP")
	}
	if !strings.Contains(listed.Body.String(), orgServerID.String()) {
		t.Fatalf("owner list missing organization MCP: %s", listed.Body.String())
	}
	listed = h.request(t, http.MethodGet, "/mcp-servers", fx.org, &fx.member, nil)
	if strings.Contains(listed.Body.String(), orgServerID.String()) {
		t.Fatalf("member list exposed organization MCP management data: %s", listed.Body.String())
	}
	if !strings.Contains(listed.Body.String(), personalServerID.String()) || !strings.Contains(listed.Body.String(), secondPersonalServerID.String()) {
		t.Fatalf("member list missing personal servers: %s", listed.Body.String())
	}
	if strings.Contains(listed.Body.String(), orgSecret) || strings.Contains(listed.Body.String(), personalSecret) {
		t.Fatal("list response leaked secrets")
	}

	firstPage := h.request(t, http.MethodGet, "/mcp-servers?limit=1", fx.org, &fx.member, nil)
	var firstPageBody struct {
		MCPServers []struct {
			ID string `json:"id"`
		} `json:"mcp_servers"`
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	}
	if err := json.Unmarshal(firstPage.Body.Bytes(), &firstPageBody); err != nil {
		t.Fatalf("decode first MCP page: %v", err)
	}
	if firstPage.Code != http.StatusOK || len(firstPageBody.MCPServers) != 1 || !firstPageBody.HasMore || firstPageBody.NextCursor == nil {
		t.Fatalf("unexpected first MCP page: code=%d body=%s", firstPage.Code, firstPage.Body.String())
	}
	secondPage := h.request(t, http.MethodGet, "/mcp-servers?limit=1&cursor="+url.QueryEscape(*firstPageBody.NextCursor), fx.org, &fx.member, nil)
	var secondPageBody struct {
		MCPServers []struct {
			ID string `json:"id"`
		} `json:"mcp_servers"`
		NextCursor *string `json:"next_cursor"`
		HasMore    bool    `json:"has_more"`
	}
	if err := json.Unmarshal(secondPage.Body.Bytes(), &secondPageBody); err != nil {
		t.Fatalf("decode second MCP page: %v", err)
	}
	if secondPage.Code != http.StatusOK || len(secondPageBody.MCPServers) != 1 || secondPageBody.HasMore || secondPageBody.NextCursor != nil || secondPageBody.MCPServers[0].ID == firstPageBody.MCPServers[0].ID {
		t.Fatalf("unexpected second MCP page: code=%d body=%s", secondPage.Code, secondPage.Body.String())
	}
	invalidPage := h.request(t, http.MethodGet, "/mcp-servers?cursor=invalid", fx.org, &fx.member, nil)
	if invalidPage.Code != http.StatusBadRequest {
		t.Fatalf("invalid MCP cursor: code=%d body=%s", invalidPage.Code, invalidPage.Body.String())
	}

	grantPath := "/orgs/current/teams/" + fx.team.ID.String() + "/mcp-servers"
	granted := h.request(t, http.MethodPost, grantPath, fx.org, &fx.owner, map[string]any{"mcp_server_id": orgServerID.String()})
	if granted.Code != http.StatusCreated {
		t.Fatalf("grant team MCP: %d %s", granted.Code, granted.Body.String())
	}
	attachPath := "/agents/" + fx.agent.ID.String() + "/personal-mcp-servers"
	attached := h.request(t, http.MethodPost, attachPath, fx.org, &fx.member, map[string]any{"mcp_server_id": personalServerID.String()})
	if attached.Code != http.StatusCreated {
		t.Fatalf("attach personal MCP: %d %s", attached.Code, attached.Body.String())
	}

	runtimeServers, err := h.service.ResolveForRuntime(t.Context(), fx.org.ID, fx.agent.ID, fx.team.ID, &fx.member.ID, true)
	if err != nil {
		t.Fatalf("resolve runtime MCPs: %v", err)
	}
	if len(runtimeServers) != 2 {
		t.Fatalf("runtime servers = %d, want 2: %+v", len(runtimeServers), runtimeServers)
	}
	headers := map[uuid.UUID]map[string]string{}
	for _, server := range runtimeServers {
		headers[server.ID] = server.Headers
	}
	if headers[orgServerID]["Authorization"] != "Bearer "+orgSecret {
		t.Fatalf("org runtime header = %#v", headers[orgServerID])
	}
	if headers[personalServerID]["X-Dev-Key"] != personalSecret {
		t.Fatalf("personal runtime header = %#v", headers[personalServerID])
	}
	withoutPersonal, err := h.service.ResolveForRuntime(t.Context(), fx.org.ID, fx.agent.ID, fx.team.ID, &fx.member.ID, false)
	if err != nil || len(withoutPersonal) != 1 || withoutPersonal[0].ID != orgServerID {
		t.Fatalf("without personal = %+v err=%v", withoutPersonal, err)
	}

	overridePath := "/agents/" + fx.agent.ID.String() + "/mcp-servers"
	disabled := h.request(t, http.MethodPut, overridePath, fx.org, &fx.owner, map[string]any{"mcp_server_id": orgServerID.String(), "state": "disabled"})
	if disabled.Code != http.StatusOK {
		t.Fatalf("disable inherited MCP: %d %s", disabled.Code, disabled.Body.String())
	}
	runtimeServers, err = h.service.ResolveForRuntime(t.Context(), fx.org.ID, fx.agent.ID, fx.team.ID, &fx.member.ID, true)
	if err != nil || len(runtimeServers) != 1 || runtimeServers[0].ID != personalServerID {
		t.Fatalf("disabled resolution = %+v err=%v", runtimeServers, err)
	}

	foreign, err := h.service.CreateServer(t.Context(), fx.otherOrg.ID, fx.otherUser.ID, mcpservers.CreateServerParams{
		Scope: model.MCPServerScopeOrg, Name: "Foreign", URL: "https://foreign.example.test/mcp",
	})
	if err != nil {
		t.Fatalf("create foreign MCP: %v", err)
	}
	foreignGet := h.request(t, http.MethodGet, "/mcp-servers/"+foreign.ID.String(), fx.org, &fx.owner, nil)
	if foreignGet.Code != http.StatusNotFound {
		t.Fatalf("foreign get = %d, want 404: %s", foreignGet.Code, foreignGet.Body.String())
	}

	var authorizations []model.MCPAuthorization
	if err := h.db.WithContext(t.Context()).Where("org_id = ?", fx.org.ID).Find(&authorizations).Error; err != nil {
		t.Fatalf("load authorizations: %v", err)
	}
	for _, authorization := range authorizations {
		ciphertext := string(authorization.CredentialsEncrypted)
		if strings.Contains(ciphertext, orgSecret) || strings.Contains(ciphertext, personalSecret) {
			t.Fatal("database ciphertext contains plaintext secret")
		}
	}
}

func TestMCPServers_AllControlPlaneEntryPoints(t *testing.T) {
	h := newMCPHarness(t, nil, "http://hivy.test/v1/mcp-servers/oauth/callback")
	fx := h.seed(t)
	created := h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.owner, map[string]any{
		"scope": "org", "name": "Lifecycle MCP", "url": "https://lifecycle.example.test/mcp",
		"auth_type": "static_bearer", "authorization_policy": "service_required",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create lifecycle MCP: %d %s", created.Code, created.Body.String())
	}
	serverID := responseMCPServerID(t, created)
	serverPath := "/mcp-servers/" + serverID.String()

	configured := h.request(t, http.MethodPut, serverPath+"/authorization", fx.org, &fx.owner, map[string]any{
		"principal_type": "org_service", "bearer_token": "entry-point-secret",
	})
	if configured.Code != http.StatusOK || strings.Contains(configured.Body.String(), "entry-point-secret") {
		t.Fatalf("configure auth: %d %s", configured.Code, configured.Body.String())
	}
	got := h.request(t, http.MethodGet, serverPath, fx.org, &fx.owner, nil)
	if got.Code != http.StatusOK {
		t.Fatalf("get MCP: %d %s", got.Code, got.Body.String())
	}
	patched := h.request(t, http.MethodPatch, serverPath, fx.org, &fx.owner, map[string]any{"description": "updated"})
	if patched.Code != http.StatusOK || !strings.Contains(patched.Body.String(), "updated") {
		t.Fatalf("patch MCP: %d %s", patched.Code, patched.Body.String())
	}

	teamPath := "/orgs/current/teams/" + fx.team.ID.String() + "/mcp-servers"
	if response := h.request(t, http.MethodPost, teamPath, fx.org, &fx.owner, map[string]any{"mcp_server_id": serverID.String()}); response.Code != http.StatusCreated {
		t.Fatalf("grant team MCP: %d %s", response.Code, response.Body.String())
	}
	if response := h.request(t, http.MethodGet, teamPath, fx.org, &fx.member, nil); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), serverID.String()) {
		t.Fatalf("list team MCP: %d %s", response.Code, response.Body.String())
	}
	if response := h.request(t, http.MethodDelete, teamPath+"/"+serverID.String(), fx.org, &fx.owner, nil); response.Code != http.StatusOK {
		t.Fatalf("revoke team MCP: %d %s", response.Code, response.Body.String())
	}

	agentPath := "/agents/" + fx.agent.ID.String() + "/mcp-servers"
	if response := h.request(t, http.MethodPut, agentPath, fx.org, &fx.owner, map[string]any{"mcp_server_id": serverID.String(), "state": "enabled"}); response.Code != http.StatusOK {
		t.Fatalf("set agent MCP: %d %s", response.Code, response.Body.String())
	}
	if response := h.request(t, http.MethodGet, agentPath, fx.org, &fx.member, nil); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), serverID.String()) {
		t.Fatalf("list agent MCP: %d %s", response.Code, response.Body.String())
	}
	if response := h.request(t, http.MethodDelete, agentPath+"/"+serverID.String(), fx.org, &fx.owner, nil); response.Code != http.StatusOK {
		t.Fatalf("delete agent MCP: %d %s", response.Code, response.Body.String())
	}

	personal := h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.member, map[string]any{
		"scope": "personal", "name": "Entry Personal", "url": "https://personal-entry.example.test/mcp", "auth_type": "none",
	})
	if personal.Code != http.StatusCreated {
		t.Fatalf("create personal entry MCP: %d %s", personal.Code, personal.Body.String())
	}
	personalID := responseMCPServerID(t, personal)
	personalPath := "/agents/" + fx.agent.ID.String() + "/personal-mcp-servers"
	if response := h.request(t, http.MethodPost, personalPath, fx.org, &fx.member, map[string]any{"mcp_server_id": personalID.String()}); response.Code != http.StatusCreated {
		t.Fatalf("attach personal MCP: %d %s", response.Code, response.Body.String())
	}
	if response := h.request(t, http.MethodGet, personalPath, fx.org, &fx.member, nil); response.Code != http.StatusOK || !strings.Contains(response.Body.String(), personalID.String()) {
		t.Fatalf("list personal MCP: %d %s", response.Code, response.Body.String())
	}
	if response := h.request(t, http.MethodDelete, personalPath+"/"+personalID.String(), fx.org, &fx.member, nil); response.Code != http.StatusOK {
		t.Fatalf("detach personal MCP: %d %s", response.Code, response.Body.String())
	}

	if response := h.request(t, http.MethodDelete, serverPath+"/authorization?principal_type=org_service", fx.org, &fx.owner, nil); response.Code != http.StatusOK {
		t.Fatalf("delete authorization: %d %s", response.Code, response.Body.String())
	}
	if response := h.request(t, http.MethodDelete, serverPath, fx.org, &fx.owner, nil); response.Code != http.StatusOK {
		t.Fatalf("delete MCP: %d %s", response.Code, response.Body.String())
	}
	metadata := h.request(t, http.MethodGet, "/v1/mcp-servers/oauth/client-metadata", fx.org, nil, nil)
	if metadata.Code != http.StatusOK || !strings.Contains(metadata.Body.String(), callbackURLPathFragment) {
		t.Fatalf("client metadata: %d %s", metadata.Code, metadata.Body.String())
	}
}

func TestMCPServers_OrgServiceAndAgentGrantsRequireManager(t *testing.T) {
	h := newMCPHarness(t, nil, "http://hivy.test/v1/mcp-servers/oauth/callback")
	fx := h.seed(t)
	created := h.request(t, http.MethodPost, "/mcp-servers", fx.org, &fx.owner, map[string]any{
		"scope": "org", "name": "Manager MCP", "url": "https://manager.example.test/mcp",
		"auth_type": "static_bearer", "authorization_policy": "service_required",
	})
	if created.Code != http.StatusCreated {
		t.Fatalf("create manager MCP: %d %s", created.Code, created.Body.String())
	}
	serverID := responseMCPServerID(t, created)
	serverPath := "/mcp-servers/" + serverID.String()

	// Omitted principal_type normalizes to org_service. The handler must perform
	// authorization after normalization so omission cannot bypass the manager gate.
	for _, request := range []struct {
		method string
		path   string
		body   any
	}{
		{http.MethodPut, serverPath + "/authorization", map[string]any{"bearer_token": "member-secret"}},
		{http.MethodDelete, serverPath + "/authorization", nil},
	} {
		response := h.request(t, request.method, request.path, fx.org, &fx.member, request.body)
		if response.Code != http.StatusForbidden {
			t.Fatalf("member org-service request %s %s = %d, want 403: %s", request.method, request.path, response.Code, response.Body.String())
		}
	}

	agentPath := "/agents/" + fx.agent.ID.String() + "/mcp-servers"
	response := h.request(t, http.MethodPut, agentPath, fx.org, &fx.member, map[string]any{
		"mcp_server_id": serverID.String(), "state": "enabled",
	})
	if response.Code != http.StatusForbidden {
		t.Fatalf("member direct agent grant = %d, want 403: %s", response.Code, response.Body.String())
	}
	response = h.request(t, http.MethodPut, serverPath+"/authorization", fx.org, &fx.owner, map[string]any{
		"principal_type": "user", "bearer_token": "wrong-principal",
	})
	if response.Code != http.StatusUnprocessableEntity {
		t.Fatalf("incompatible user authorization = %d, want 422: %s", response.Code, response.Body.String())
	}
}
