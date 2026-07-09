package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_ChannelsCreate_AllowsSameNameAcrossSources(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	base := map[string]any{
		"name":             "#engineering",
		"category":         "engineering",
		"team_id":          fx.team.ID.String(),
		"default_agent_id": fx.agent.ID.String(),
	}

	native := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, base)
	if native.Code != http.StatusCreated {
		t.Fatalf("native status=%d body=%s", native.Code, native.Body.String())
	}
	nativeOut := decodeChannelCreate(t, native)
	if nativeOut.Channel.Name != "engineering" || nativeOut.Channel.Origin != "native" {
		t.Fatalf("native channel source/name mismatch: %+v", nativeOut.Channel)
	}
	slackConn := seedChannelConnection(t, h, fx, "slack", "slack-workspace-1")

	slack := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#engineering",
		"team_id":                fx.team.ID.String(),
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "slack",
		"external_connection_id": slackConn.ID.String(),
		"external_workspace_key": "T123",
		"external_resource_type": "slack_channel",
		"external_resource_key":  "CENG",
		"external_resource_name": "engineering",
		"external_resource_url":  "https://slack.test/T123/CENG",
		"external_metadata":      map[string]any{"is_private": false},
	})
	if slack.Code != http.StatusCreated {
		t.Fatalf("slack status=%d body=%s", slack.Code, slack.Body.String())
	}
	slackOut := decodeChannelCreate(t, slack)
	if slackOut.Channel.Origin != "external" || slackOut.Channel.ExternalProvider != "slack" {
		t.Fatalf("slack channel source mismatch: %+v", slackOut.Channel)
	}
	if slackOut.Channel.Name != "slack-engineering" || slackOut.Channel.ExternalResourceKey != "CENG" {
		t.Fatalf("slack channel name/resource mismatch: %+v", slackOut.Channel)
	}
	discordConn := seedChannelConnection(t, h, fx, "discord", "discord-workspace-1")

	discord := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#engineering",
		"team_id":                fx.team.ID.String(),
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "discord",
		"external_connection_id": discordConn.ID.String(),
		"external_workspace_key": "GUILD-1",
		"external_resource_key":  "987",
	})
	if discord.Code != http.StatusCreated {
		t.Fatalf("discord status=%d body=%s", discord.Code, discord.Body.String())
	}
	discordOut := decodeChannelCreate(t, discord)
	if discordOut.Channel.Name != "discord-engineering" {
		t.Fatalf("discord channel name=%q, want provider prefix", discordOut.Channel.Name)
	}

	duplicateSlackName := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#engineering",
		"team_id":                fx.team.ID.String(),
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "slack",
		"external_connection_id": slackConn.ID.String(),
		"external_workspace_key": "T123",
		"external_resource_key":  "COTHER",
	})
	if duplicateSlackName.Code != http.StatusConflict {
		t.Fatalf("duplicate slack same source/name status=%d body=%s", duplicateSlackName.Code, duplicateSlackName.Body.String())
	}

	duplicateSlackResource := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#platform",
		"team_id":                fx.team.ID.String(),
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "slack",
		"external_connection_id": slackConn.ID.String(),
		"external_workspace_key": "T123",
		"external_resource_key":  "CENG",
	})
	if duplicateSlackResource.Code != http.StatusConflict {
		t.Fatalf("duplicate slack resource status=%d body=%s", duplicateSlackResource.Code, duplicateSlackResource.Body.String())
	}

	var count int64
	if err := h.db.Model(&model.Channel{}).
		Where("org_id = ? AND name = ?", fx.org.ID, "engineering").
		Count(&count).Error; err != nil {
		t.Fatalf("count channels: %v", err)
	}
	if count != 1 {
		t.Fatalf("engineering channel count=%d, want 1 native channel", count)
	}
}

func TestIntegration_ChannelsVisibilityAndJoin(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	publicID := createChannelForTest(t, h, fx, fx.owner, "ops", "public")
	addChannelTeamMember(t, h, fx, fx.member, fx.team.ID)
	team := seedChannelTeam(t, h, fx, "Leadership")
	privateID := createTeamChannelForTest(t, h, fx, "leadership", team.ID)

	privateGet := h.doJSON(t, http.MethodGet, "/v1/channels/"+privateID, fx, fx.member, nil)
	if privateGet.Code != http.StatusForbidden {
		t.Fatalf("private get status=%d body=%s", privateGet.Code, privateGet.Body.String())
	}

	memberList := h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil)
	assertChannelNames(t, memberList, []string{"ops"})
	discoverable := h.doJSON(t, http.MethodGet, "/v1/channels?discoverable=true", fx, fx.member, nil)
	assertChannelNames(t, discoverable, []string{"ops"})

	joined := h.doJSON(t, http.MethodPost, "/v1/channels/"+publicID+"/join", fx, fx.member, nil)
	if joined.Code != http.StatusOK {
		t.Fatalf("join status=%d body=%s", joined.Code, joined.Body.String())
	}
	memberList = h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil)
	assertChannelNames(t, memberList, []string{"ops"})
	if privateID == "" {
		t.Fatal("private channel was not created")
	}
}

func TestIntegration_ChannelsListIsMembershipScoped(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	publicID := createChannelForTest(t, h, fx, fx.owner, "general", "public")
	addChannelTeamMember(t, h, fx, fx.member, fx.team.ID)
	engineering := seedChannelTeam(t, h, fx, "Engineering")
	engineeringID := createTeamChannelForTest(t, h, fx, "engineering", engineering.ID)
	privateTeam := seedChannelTeam(t, h, fx, "Private")
	privateID := createTeamChannelForTest(t, h, fx, "private-work", privateTeam.ID)

	// A member sees only channels of teams they belong to.
	assertChannelNameSet(t, h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil), []string{"general"})

	// Joining a team reveals that team's channel.
	if err := h.db.Create(&model.TeamMember{OrgID: fx.org.ID, TeamID: engineering.ID, UserID: fx.member.ID, Role: "member"}).Error; err != nil {
		t.Fatalf("add team member: %v", err)
	}
	assertChannelNameSet(t, h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil), []string{"general", "engineering"})

	// Merely participating in a session does NOT grant access to another team's channel.
	privateUUID := uuid.MustParse(privateID)
	seedChannelRecentSession(t, h, fx, privateUUID, fx.owner.ID, []uuid.UUID{fx.member.ID}, time.Now())
	assertChannelNameSet(t, h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil), []string{"general", "engineering"})

	privateGet := h.doJSON(t, http.MethodGet, "/v1/channels/"+privateID, fx, fx.member, nil)
	if privateGet.Code != http.StatusForbidden {
		t.Fatalf("non-member channel get status=%d body=%s", privateGet.Code, privateGet.Body.String())
	}

	extChannel := model.Channel{
		OrgID:            fx.org.ID,
		TeamID:           privateTeam.ID,
		Name:             "slack-qa",
		Kind:             "standard",
		Visibility:       "public",
		Origin:           "external",
		ExternalProvider: "slack",
		DefaultAgentID:   fx.agent.ID,
	}
	if err := h.db.Create(&extChannel).Error; err != nil {
		t.Fatalf("create external channel: %v", err)
	}
	assertChannelNameSet(t, h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil), []string{"general", "engineering"})
	extGet := h.doJSON(t, http.MethodGet, "/v1/channels/"+extChannel.ID.String(), fx, fx.member, nil)
	if extGet.Code != http.StatusForbidden {
		t.Fatalf("external non-member channel get status=%d body=%s", extGet.Code, extGet.Body.String())
	}

	if publicID == "" || engineeringID == "" {
		t.Fatal("expected channels to be created")
	}
}

func TestIntegration_ChannelsListRecentSessionsShowAllForMembers(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channelID := createChannelForTest(t, h, fx, fx.owner, "customer-work", "public")
	channelUUID := uuid.MustParse(channelID)
	base := time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)

	// Six sessions from mixed creators, none with the caller as participant.
	// A channel member sees them all, newest activity first.
	sessions := make([]model.Session, 0, 6)
	for i := 0; i < 6; i++ {
		creator := fx.owner.ID
		if i%2 == 0 {
			creator = fx.member.ID
		}
		sessions = append(sessions, seedChannelRecentSession(t, h, fx, channelUUID, creator, nil, base.Add(time.Duration(i)*time.Hour)))
	}

	rr := h.doJSON(t, http.MethodGet, "/v1/channels?include=recent_sessions&recent_sessions_limit=5", fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out channelListOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v\n%s", err, rr.Body.String())
	}

	var channel channelOut
	for _, entry := range out.Data {
		if entry.ID == channelID {
			channel = entry
			break
		}
	}
	if channel.ID == "" {
		t.Fatalf("channel %s not found in response: %+v", channelID, out.Data)
	}
	if len(channel.RecentSessions) != 5 {
		t.Fatalf("recent sessions len=%d, want 5: %+v", len(channel.RecentSessions), channel.RecentSessions)
	}
	if !channel.RecentSessionsHasMore || channel.RecentSessionsNextCursor == nil || *channel.RecentSessionsNextCursor == "" {
		t.Fatalf("expected recent sessions next cursor, got has_more=%v cursor=%v", channel.RecentSessionsHasMore, channel.RecentSessionsNextCursor)
	}
	wantIDs := []string{
		sessions[5].ID.String(),
		sessions[4].ID.String(),
		sessions[3].ID.String(),
		sessions[2].ID.String(),
		sessions[1].ID.String(),
	}
	for i, session := range channel.RecentSessions {
		if session.ID != wantIDs[i] {
			gotIDs := make([]string, len(channel.RecentSessions))
			for j, s := range channel.RecentSessions {
				gotIDs[j] = s.ID
			}
			t.Fatalf("recent session ids=%v, want=%v", gotIDs, wantIDs)
		}
	}
}

func TestIntegration_ChannelsManagementGuardrails(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channelID := createChannelForTest(t, h, fx, fx.owner, "platform", "public")

	memberPatch := h.doJSON(t, http.MethodPatch, "/v1/channels/"+channelID, fx, fx.member, map[string]any{
		"description": "member attempted edit",
	})
	if memberPatch.Code != http.StatusForbidden {
		t.Fatalf("member patch status=%d body=%s", memberPatch.Code, memberPatch.Body.String())
	}

	ownerPatch := h.doJSON(t, http.MethodPatch, "/v1/channels/"+channelID, fx, fx.owner, map[string]any{
		"name":        "platform-team",
		"description": "owner edit",
	})
	if ownerPatch.Code != http.StatusOK {
		t.Fatalf("owner patch status=%d body=%s", ownerPatch.Code, ownerPatch.Body.String())
	}
	out := decodeChannelCreate(t, ownerPatch)
	if out.Channel.Name != "platform-team" {
		t.Fatalf("patched name=%q", out.Channel.Name)
	}

	generalID := seedDefaultChannel(t, h, fx)
	deleteDefault := h.doJSON(t, http.MethodDelete, "/v1/channels/"+generalID, fx, fx.owner, nil)
	if deleteDefault.Code != http.StatusBadRequest {
		t.Fatalf("delete default status=%d body=%s", deleteDefault.Code, deleteDefault.Body.String())
	}
}
