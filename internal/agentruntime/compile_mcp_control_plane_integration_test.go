package agentruntime

import (
	"bytes"
	"context"
	"encoding/base64"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/mcpservers"
	"github.com/usehivy/hivy/internal/model"
)

func TestCompileWithProxyTokenOptions_ResolvesControlPlaneMCPServersByActorAndSource(t *testing.T) {
	db := connectCompileTestDB(t)
	ctx := context.Background()
	org := model.Org{Name: "runtime-mcp-" + uuid.NewString()}
	if err := db.WithContext(ctx).Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { _ = db.WithContext(ctx).Where("id = ?", org.ID).Delete(&model.Org{}).Error })
	team := seedCompileTeam(t, db, org.ID)
	actor := model.User{Email: "runtime-mcp-" + uuid.NewString() + "@example.com", Name: "Runtime MCP actor"}
	if err := db.WithContext(ctx).Create(&actor).Error; err != nil {
		t.Fatalf("create actor: %v", err)
	}
	if err := db.WithContext(ctx).Create(&model.OrgMembership{OrgID: org.ID, UserID: actor.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("create actor org membership: %v", err)
	}
	if err := db.WithContext(ctx).Create(&model.TeamMember{OrgID: org.ID, TeamID: team.ID, UserID: actor.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("create actor team membership: %v", err)
	}
	agent := model.Agent{
		ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "MCP agent", Model: DefaultAgentModel,
		Tools: model.JSON{}, McpServers: model.RawJSON(`[{"name":"legacy-bypass","transport":"streamable_http","url":"https://legacy.example.com"}]`),
		Skills: model.JSON{}, Integrations: model.JSON{}, Resources: model.JSON{}, RuntimeConfig: model.JSON{}, Permissions: model.JSON{},
		Status: "active",
	}
	if err := db.WithContext(ctx).Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}

	key, err := crypto.NewSymmetricKey(base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{0x42}, 32)))
	if err != nil {
		t.Fatalf("create symmetric key: %v", err)
	}
	service := mcpservers.NewService(db, key, "")
	orgServer, err := service.CreateServer(ctx, org.ID, actor.ID, mcpservers.CreateServerParams{
		Scope: model.MCPServerScopeOrg, Name: "Organization tools", Slug: "org-tools",
		URL: "https://org-mcp.example.com/mcp", AuthType: model.MCPAuthTypeNone,
	})
	if err != nil {
		t.Fatalf("create org MCP server: %v", err)
	}
	if err := service.GrantTeam(ctx, org.ID, team.ID, orgServer.ID, &actor.ID); err != nil {
		t.Fatalf("grant org MCP server to team: %v", err)
	}
	personalServer, err := service.CreateServer(ctx, org.ID, actor.ID, mcpservers.CreateServerParams{
		Scope: model.MCPServerScopePersonal, Name: "Personal tools", Slug: "personal-tools",
		URL: "https://personal-mcp.example.com/mcp", AuthType: model.MCPAuthTypeStaticBearer,
		Authorization: &mcpservers.AuthorizationInput{PrincipalType: model.MCPPrincipalUser, BearerToken: "personal-secret"},
	})
	if err != nil {
		t.Fatalf("create personal MCP server: %v", err)
	}
	if err := service.AttachPersonal(ctx, org.ID, actor.ID, agent.ID, personalServer.ID); err != nil {
		t.Fatalf("attach personal MCP server: %v", err)
	}

	deps := CompileDeps{DB: db, EncKey: key, Cfg: &config.Config{}}
	proxyToken := &ProxyTokenResult{Token: "ptok_test", JTI: "runtime-mcp-test"}
	assertNames := func(source string, actorID *uuid.UUID, want ...string) *AgentDefinition {
		t.Helper()
		definition, err := CompileWithProxyTokenOptions(ctx, deps, &agent, proxyToken, RuntimeConfigOptions{
			TeamID: team.ID, MCPContext: MCPRuntimeContext{ActorUserID: actorID, Source: source},
		})
		if err != nil {
			t.Fatalf("compile source %q: %v", source, err)
		}
		got := map[string]bool{}
		for _, raw := range definition.McpServers {
			server, ok := raw.(map[string]any)
			if !ok {
				t.Fatalf("MCP server has type %T", raw)
			}
			name, _ := server["name"].(string)
			got[name] = true
		}
		if len(got) != len(want) {
			t.Fatalf("source %q MCP names = %#v, want %v", source, got, want)
		}
		for _, name := range want {
			if !got[name] {
				t.Fatalf("source %q missing MCP server %q in %#v", source, name, got)
			}
		}
		if got["legacy-bypass"] {
			t.Fatal("legacy agent mcp_servers JSON bypassed the MCP control plane")
		}
		return definition
	}

	webDefinition := assertNames(MCPInvocationWeb, &actor.ID, "org-tools", "personal-tools")
	for _, raw := range webDefinition.McpServers {
		server := raw.(map[string]any)
		if server["name"] == "personal-tools" {
			headers := server["headers"].(map[string]string)
			if headers["Authorization"] != "Bearer personal-secret" {
				t.Fatalf("personal authorization header = %q", headers["Authorization"])
			}
		}
	}
	assertNames(MCPInvocationSchedule, &actor.ID, "org-tools", "personal-tools")
	assertNames(MCPInvocationCron, &actor.ID, "org-tools", "personal-tools")
	assertNames(model.SessionSourceExternal, &actor.ID, "org-tools")
	assertNames("automation", &actor.ID, "org-tools")
	assertNames("webhook", &actor.ID, "org-tools")
	assertNames(MCPInvocationWeb, nil, "org-tools")
	versionBeforeDeactivation, err := MCPConfigVersion(ctx, db, org.ID)
	if err != nil {
		t.Fatalf("load MCP config version before deactivation: %v", err)
	}
	if err := db.WithContext(ctx).Model(&model.OrgMembership{}).
		Where("org_id = ? AND user_id = ?", org.ID, actor.ID).
		Update("deactivated_at", db.NowFunc()).Error; err != nil {
		t.Fatalf("deactivate actor membership: %v", err)
	}
	versionAfterDeactivation, err := MCPConfigVersion(ctx, db, org.ID)
	if err != nil {
		t.Fatalf("load MCP config version after deactivation: %v", err)
	}
	if versionAfterDeactivation <= versionBeforeDeactivation {
		t.Fatalf("membership revocation did not advance MCP config version: before=%d after=%d", versionBeforeDeactivation, versionAfterDeactivation)
	}
	assertNames(MCPInvocationSchedule, &actor.ID, "org-tools")
}
