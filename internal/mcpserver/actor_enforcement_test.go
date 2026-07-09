package mcpserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// actorEnforcementFixture extends the channel-tool fixture with a team-scoped
// private channel plus two users: a plain org member who is NOT on that team,
// and an org admin. It exercises the per-user access checks wired into the
// agent-facing MCP tools.
type actorEnforcementFixture struct {
	channelToolFixture
	teamChannel model.Channel // team-scoped; member is not on the team
	member      model.User    // org role "member", not on the team
	admin       model.User    // org role "admin"
}

func seedActorEnforcementFixture(t *testing.T, db *gorm.DB) actorEnforcementFixture {
	t.Helper()
	base := seedChannelToolFixture(t, db)

	teamChannel := model.Channel{
		ID:             uuid.New(),
		OrgID:          base.org.ID,
		Name:           "private-" + uuid.NewString(),
		DefaultAgentID: base.agent.ID,
		Visibility:     "private",
		TeamID:         &base.team.ID,
	}
	member := model.User{ID: uuid.New(), Email: "member-" + uuid.NewString() + "@example.test", Name: "Member"}
	admin := model.User{ID: uuid.New(), Email: "admin-" + uuid.NewString() + "@example.test", Name: "Admin"}
	memberMembership := model.OrgMembership{UserID: member.ID, OrgID: base.org.ID, Role: "member"}
	adminMembership := model.OrgMembership{UserID: admin.ID, OrgID: base.org.ID, Role: "admin"}

	for _, row := range []any{&teamChannel, &member, &admin, &memberMembership, &adminMembership} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed actor fixture row %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", base.org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", base.org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", base.org.ID).Delete(&model.OrgMembership{})
		db.Delete(&model.User{}, "id IN ?", []uuid.UUID{member.ID, admin.ID})
	})
	return actorEnforcementFixture{channelToolFixture: base, teamChannel: teamChannel, member: member, admin: admin}
}

func decodeToolError(t *testing.T, result *mcp.CallToolResult) string {
	t.Helper()
	if result == nil || !result.IsError || len(result.Content) == 0 {
		t.Fatalf("expected an error result, got %+v", result)
	}
	text, ok := result.Content[0].(*mcp.TextContent)
	if !ok {
		t.Fatalf("unexpected content type %T", result.Content[0])
	}
	return text.Text
}

func TestListChannelsHidesChannelsActorCannotUse(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedActorEnforcementFixture(t, db)

	// A plain member who is not on the channel's team must not see it.
	result, err := handleListChannels(t.Context(), db, &fx.agent, fx.member.ID.String())
	if err != nil {
		t.Fatalf("handleListChannels: %v", err)
	}
	names := channelNamesFromResult(t, result)
	if names[fx.teamChannel.Name] {
		t.Fatalf("member should not see team channel %q; got %v", fx.teamChannel.Name, names)
	}
	if !names[fx.general.Name] {
		t.Fatalf("member should still see the general channel; got %v", names)
	}

	// An org admin sees everything.
	adminResult, err := handleListChannels(t.Context(), db, &fx.agent, fx.admin.ID.String())
	if err != nil {
		t.Fatalf("handleListChannels admin: %v", err)
	}
	adminNames := channelNamesFromResult(t, adminResult)
	if !adminNames[fx.teamChannel.Name] {
		t.Fatalf("admin should see team channel %q; got %v", fx.teamChannel.Name, adminNames)
	}

	// No actor (automated run) keeps the prior agent-scoped behavior: all channels.
	autoResult, err := handleListChannels(t.Context(), db, &fx.agent, "")
	if err != nil {
		t.Fatalf("handleListChannels no-actor: %v", err)
	}
	if !channelNamesFromResult(t, autoResult)[fx.teamChannel.Name] {
		t.Fatal("automated run should keep agent-scoped visibility of all channels")
	}
}

func TestCronBlocksBindingChannelActorCannotUse(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedActorEnforcementFixture(t, db)

	args := cronToolArgs{
		Action:          "create",
		TaskPrompt:      "do a thing",
		IntervalSeconds: ptrInt64(3600),
		ChannelID:       fx.teamChannel.ID.String(),
		HivyActorUserID: fx.member.ID.String(),
	}
	result, err := handleCronTool(t.Context(), db, &fx.agent, args)
	if err != nil {
		t.Fatalf("handleCronTool: %v", err)
	}
	msg := decodeToolError(t, result)
	if !strings.Contains(msg, "channels you belong to") {
		t.Fatalf("expected a clear channel-membership error, got %q", msg)
	}

	// The admin passes the channel-access gate. (The create may still fail on
	// unrelated schedule plumbing that needs a live session; we only assert the
	// access check itself does not block the admin.)
	args.HivyActorUserID = fx.admin.ID.String()
	adminResult, err := handleCronTool(t.Context(), db, &fx.agent, args)
	if err != nil {
		t.Fatalf("handleCronTool admin: %v", err)
	}
	if adminResult.IsError {
		if adminMsg := adminResult.Content[0].(*mcp.TextContent).Text; strings.Contains(adminMsg, "channels you belong to") {
			t.Fatalf("admin should not be blocked by the channel-access check: %q", adminMsg)
		}
	}
}

func TestCronCreateBlocksCrossTeamTargeting(t *testing.T) {
	db := connectChannelToolTestDB(t)
	fx := seedActorEnforcementFixture(t, db)

	otherAgent := model.Agent{ID: uuid.New(), OrgID: &fx.org.ID, TeamID: fx.team.ID, Name: "Other " + uuid.NewString(), Model: "test", Status: "active"}
	if err := db.Create(&otherAgent).Error; err != nil {
		t.Fatalf("seed other agent: %v", err)
	}

	// Targeting an agent on a team the actor doesn't belong to is denied, even
	// when the named channel itself is usable.
	crossTeam, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{
		Action:          "create",
		AgentID:         otherAgent.ID.String(),
		TaskPrompt:      "do a thing",
		IntervalSeconds: ptrInt64(3600),
		ChannelID:       fx.general.ID.String(),
		HivyActorUserID: fx.member.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCronTool cross-team: %v", err)
	}
	if msg := decodeToolError(t, crossTeam); !strings.Contains(msg, "teams you belong to") {
		t.Fatalf("expected cross-team agent denial, got %q", msg)
	}

	// An empty channel_id defaults to the agent's team #general; a member who
	// isn't on that team can't use it, so the create is denied.
	emptyChan, err := handleCronTool(t.Context(), db, &fx.agent, cronToolArgs{
		Action:          "create",
		TaskPrompt:      "do a thing",
		IntervalSeconds: ptrInt64(3600),
		HivyActorUserID: fx.member.ID.String(),
	})
	if err != nil {
		t.Fatalf("handleCronTool empty-channel: %v", err)
	}
	if msg := decodeToolError(t, emptyChan); !strings.Contains(msg, "channels you belong to") {
		t.Fatalf("expected default-channel denial, got %q", msg)
	}
}

func channelNamesFromResult(t *testing.T, result *mcp.CallToolResult) map[string]bool {
	t.Helper()
	if result == nil || result.IsError || len(result.Content) == 0 {
		t.Fatalf("unexpected list_channels result: %+v", result)
	}
	text := result.Content[0].(*mcp.TextContent).Text
	var payload struct {
		Channels []struct {
			Name string `json:"name"`
		} `json:"channels"`
	}
	if err := json.Unmarshal([]byte(text), &payload); err != nil {
		t.Fatalf("decode channels %q: %v", text, err)
	}
	names := map[string]bool{}
	for _, c := range payload.Channels {
		names[c.Name] = true
	}
	return names
}

func ptrInt64(v int64) *int64 { return &v }
