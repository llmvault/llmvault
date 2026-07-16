package mcpserver

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/mcp/catalog"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

func TestBuildConnectionServerListsOnlyConnectionProviderActions(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedChannelToolFixture(t, db)
	user := model.User{ID: uuid.New(), Email: "connection-mcp-" + uuid.NewString() + "@example.test", Name: "Connection owner"}
	integration := model.Integration{ID: uuid.New(), UniqueKey: "slack-" + uuid.NewString(), Provider: "slack", DisplayName: "Slack"}
	connection := model.Connection{ID: uuid.New(), OrgID: fx.org.ID, UserID: user.ID, IntegrationID: integration.ID, NangoConnectionID: "nango-slack", Name: "Support", Slug: "support"}
	grant := model.TeamConnectionGrant{ID: uuid.New(), OrgID: fx.org.ID, TeamID: fx.team.ID, ConnectionID: &connection.ID}
	for _, row := range []any{&user, &integration, &connection, &grant} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed connection MCP row %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Delete(&connection)
		db.Delete(&grant)
		db.Delete(&integration)
		db.Delete(&user)
	})

	server, err := BuildConnectionServer(t.Context(), db, nango.NewClient("http://nango.invalid", "test"), catalog.Global(), agentProxyToken(fx), connection.ID)
	if err != nil {
		t.Fatalf("build connection server: %v", err)
	}
	names := listServerToolNames(t, server)
	if !names["conversations_history"] || !names["chat_post_message"] {
		t.Fatalf("Slack tools missing from tools/list: %v", names)
	}
	if names["issues_create"] {
		t.Fatalf("connection server leaked another provider's tool: %v", names)
	}
}
