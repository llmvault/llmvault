package mcpserver

import (
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

func toolErrorText(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || !result.IsError || len(result.Content) == 0 {
		t.Fatalf("expected error result, got %#v", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type %T", result.Content[0])
	}
	return text.Text
}

func TestCreateHTTPTriggerDefaultsToTeamGeneral(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedChannelToolFixture(t, db)

	result, err := handleCreateHTTPTrigger(t.Context(), db, agentProxyToken(fx), &fx.agent, httpTriggerArgs{
		Instructions: "summarize the payload",
	})
	if err != nil {
		t.Fatalf("handleCreateHTTPTrigger: %v", err)
	}
	out := decodeChannelToolResult(t, result)
	triggerID, _ := out["trigger_id"].(string)
	if triggerID == "" {
		t.Fatalf("trigger_id missing in %v", out)
	}

	var trigger model.AgentTrigger
	if err := db.First(&trigger, "id = ?", uuid.MustParse(triggerID)).Error; err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	if trigger.ChannelID == nil {
		t.Fatal("trigger channel is nil, want the team #general channel")
	}
	// An empty channel_id resolves to the agent's team #general (team-scoped,
	// IsDefault), never a system channel.
	if *trigger.ChannelID != fx.teamHome.ID {
		t.Fatalf("trigger channel = %s, want team #general %s", trigger.ChannelID, fx.teamHome.ID)
	}
	var channel model.Channel
	if err := db.First(&channel, "id = ?", *trigger.ChannelID).Error; err != nil {
		t.Fatalf("load trigger channel: %v", err)
	}
	if channel.OrgID != fx.org.ID || channel.TeamID == nil || *channel.TeamID != fx.team.ID || !channel.IsDefault {
		t.Fatalf("trigger channel = %#v, want team #general", channel)
	}
	// No system channel exists in the org at all.
	var systemCount int64
	if err := db.Model(&model.Channel{}).Where("org_id = ? AND kind = ?", fx.org.ID, "system").Count(&systemCount).Error; err != nil {
		t.Fatalf("count system channels: %v", err)
	}
	if systemCount != 0 {
		t.Fatalf("found %d system channels, want 0", systemCount)
	}
}

func TestCreateHTTPTriggerRejectsProviderChannelID(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedChannelToolFixture(t, db)

	result, err := handleCreateHTTPTrigger(t.Context(), db, agentProxyToken(fx), &fx.agent, httpTriggerArgs{
		Instructions: "summarize the payload",
		ChannelID:    "C0123ABCD",
	})
	if err != nil {
		t.Fatalf("handleCreateHTTPTrigger: %v", err)
	}
	if text := toolErrorText(t, result); !strings.Contains(text, "channel_id must be a uuid") {
		t.Fatalf("error = %q, want channel_id must be a uuid", text)
	}
}

func TestCreateHTTPTriggerRejectsWrongOrgChannel(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedChannelToolFixture(t, db)
	other := seedChannelToolFixture(t, db)

	result, err := handleCreateHTTPTrigger(t.Context(), db, agentProxyToken(fx), &fx.agent, httpTriggerArgs{
		Instructions: "summarize the payload",
		ChannelID:    other.general.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCreateHTTPTrigger: %v", err)
	}
	if text := toolErrorText(t, result); !strings.Contains(text, "channel_id not found") {
		t.Fatalf("error = %q, want channel_id not found", text)
	}
}

func TestCreateHTTPTriggerUsesSelectedChannelAndAgentRestrictions(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedChannelToolFixture(t, db)
	// A channel owned by a team the agent does not belong to: the team-primary
	// rule (channelagents.ActsInChannel) that replaced the cut agent_channels
	// allowlist must reject it.
	foreignTeam := model.Team{ID: uuid.New(), OrgID: fx.org.ID, Name: "foreign-" + uuid.NewString()}
	if err := db.Create(&foreignTeam).Error; err != nil {
		t.Fatalf("create foreign team: %v", err)
	}
	foreignChannel := model.Channel{ID: uuid.New(), OrgID: fx.org.ID, TeamID: &foreignTeam.ID, Name: "foreign-" + uuid.NewString(), DefaultAgentID: fx.agent.ID}
	if err := db.Create(&foreignChannel).Error; err != nil {
		t.Fatalf("create foreign channel: %v", err)
	}

	// Disallowed channel is rejected by the shared access rule.
	result, err := handleCreateHTTPTrigger(t.Context(), db, agentProxyToken(fx), &fx.agent, httpTriggerArgs{
		Instructions: "summarize the payload",
		ChannelID:    foreignChannel.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCreateHTTPTrigger: %v", err)
	}
	if text := toolErrorText(t, result); !strings.Contains(text, "agent is not available in this channel") {
		t.Fatalf("error = %q, want agent restriction", text)
	}

	// Allowed channel is stored as-is.
	result, err = handleCreateHTTPTrigger(t.Context(), db, agentProxyToken(fx), &fx.agent, httpTriggerArgs{
		Instructions: "summarize the payload",
		ChannelID:    fx.general.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCreateHTTPTrigger: %v", err)
	}
	out := decodeChannelToolResult(t, result)
	var trigger model.AgentTrigger
	if err := db.First(&trigger, "id = ?", uuid.MustParse(out["trigger_id"].(string))).Error; err != nil {
		t.Fatalf("load trigger: %v", err)
	}
	if trigger.ChannelID == nil || *trigger.ChannelID != fx.general.ID {
		t.Fatalf("trigger channel = %v, want %s", trigger.ChannelID, fx.general.ID)
	}
}
