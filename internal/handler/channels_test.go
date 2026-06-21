package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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

	slack := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#engineering",
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "slack",
		"external_workspace_key": "T123",
		"external_resource_type": "channel",
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

	discord := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#engineering",
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "discord",
		"external_workspace_key": "GUILD-1",
		"external_resource_key":  "987",
	})
	if discord.Code != http.StatusCreated {
		t.Fatalf("discord status=%d body=%s", discord.Code, discord.Body.String())
	}

	duplicateSlackName := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#engineering",
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "slack",
		"external_workspace_key": "T123",
		"external_resource_key":  "COTHER",
	})
	if duplicateSlackName.Code != http.StatusConflict {
		t.Fatalf("duplicate slack same source/name status=%d body=%s", duplicateSlackName.Code, duplicateSlackName.Body.String())
	}

	duplicateSlackResource := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":                   "#platform",
		"default_agent_id":       fx.agent.ID.String(),
		"external_provider":      "slack",
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
	if count != 3 {
		t.Fatalf("engineering channel count=%d, want 3", count)
	}
}

func TestIntegration_ChannelsVisibilityAndJoin(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	publicID := createChannelForTest(t, h, fx, fx.owner, "ops", "public")
	privateID := createChannelForTest(t, h, fx, fx.owner, "leadership", "private")

	privateGet := h.doJSON(t, http.MethodGet, "/v1/channels/"+privateID, fx, fx.member, nil)
	if privateGet.Code != http.StatusForbidden {
		t.Fatalf("private get status=%d body=%s", privateGet.Code, privateGet.Body.String())
	}

	memberList := h.doJSON(t, http.MethodGet, "/v1/channels", fx, fx.member, nil)
	assertChannelNames(t, memberList, nil)
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

func TestIntegration_ChannelsListIncludesRecentSessionsForCurrentUser(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)
	channelID := createChannelForTest(t, h, fx, fx.owner, "customer-work", "public")
	channelUUID := uuid.MustParse(channelID)
	base := time.Date(2026, 6, 21, 8, 0, 0, 0, time.UTC)

	oldCreatedByOwner := seedChannelRecentSession(t, h, fx, channelUUID, fx.owner.ID, nil, base.Add(-6*time.Hour))
	hidden := seedChannelRecentSession(t, h, fx, channelUUID, fx.member.ID, []uuid.UUID{fx.member.ID}, base.Add(2*time.Hour))
	participantSessions := make([]model.Session, 0, 5)
	for i := 0; i < 5; i++ {
		session := seedChannelRecentSession(t, h, fx, channelUUID, fx.member.ID, []uuid.UUID{fx.owner.ID}, base.Add(time.Duration(i)*time.Hour))
		participantSessions = append(participantSessions, session)
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
	gotIDs := make([]string, len(channel.RecentSessions))
	for i, session := range channel.RecentSessions {
		gotIDs[i] = session.ID
		if session.ID == hidden.ID.String() {
			t.Fatalf("hidden session was included: %+v", channel.RecentSessions)
		}
	}
	wantIDs := []string{
		participantSessions[4].ID.String(),
		participantSessions[3].ID.String(),
		participantSessions[2].ID.String(),
		participantSessions[1].ID.String(),
		participantSessions[0].ID.String(),
	}
	for i := range wantIDs {
		if gotIDs[i] != wantIDs[i] {
			t.Fatalf("recent session ids=%v, want=%v", gotIDs, wantIDs)
		}
	}
	if oldCreatedByOwner.ID == uuid.Nil {
		t.Fatal("created-by owner session was not seeded")
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

func TestIntegration_ChannelsRejectReservedSystemName(t *testing.T) {
	h := newChannelHarness(t)
	fx := h.seed(t)

	create := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":             "#system",
		"default_agent_id": fx.agent.ID.String(),
	})
	if create.Code != http.StatusBadRequest {
		t.Fatalf("create system status=%d body=%s", create.Code, create.Body.String())
	}

	channelID := createChannelForTest(t, h, fx, fx.owner, "ops", "public")
	rename := h.doJSON(t, http.MethodPatch, "/v1/channels/"+channelID, fx, fx.owner, map[string]any{
		"name": "system",
	})
	if rename.Code != http.StatusBadRequest {
		t.Fatalf("rename system status=%d body=%s", rename.Code, rename.Body.String())
	}
}

func createChannelForTest(t *testing.T, h *channelHarness, fx channelFixture, user model.User, name, visibility string) string {
	t.Helper()
	rr := h.doJSON(t, http.MethodPost, "/v1/channels", fx, user, map[string]any{
		"name":             name,
		"visibility":       visibility,
		"default_agent_id": fx.agent.ID.String(),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create channel %s status=%d body=%s", name, rr.Code, rr.Body.String())
	}
	out := decodeChannelCreate(t, rr)
	return out.Channel.ID
}

func assertChannelNames(t *testing.T, rr *httptest.ResponseRecorder, want []string) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out channelListOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v\n%s", err, rr.Body.String())
	}
	got := make([]string, len(out.Data))
	for i, channel := range out.Data {
		got[i] = channel.Name
	}
	if len(got) != len(want) {
		t.Fatalf("channel names=%v, want=%v", got, want)
	}
	for i := range got {
		if got[i] != want[i] {
			t.Fatalf("channel names=%v, want=%v", got, want)
		}
	}
}

func seedDefaultChannel(t *testing.T, h *channelHarness, fx channelFixture) string {
	t.Helper()
	channel := model.Channel{
		OrgID:          fx.org.ID,
		Name:           "general-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: fx.agent.ID,
		IsDefault:      true,
		Origin:         "native",
		CreatedBy:      &fx.owner.ID,
	}
	if err := h.db.Create(&channel).Error; err != nil {
		t.Fatalf("create default channel: %v", err)
	}
	member := model.ChannelMember{ChannelID: channel.ID, UserID: fx.owner.ID, Role: "owner", CreatedAt: time.Now()}
	if err := h.db.Create(&member).Error; err != nil {
		t.Fatalf("create default channel member: %v", err)
	}
	return channel.ID.String()
}

func seedChannelRecentSession(t *testing.T, h *channelHarness, fx channelFixture, channelID uuid.UUID, createdBy uuid.UUID, participantIDs []uuid.UUID, activityAt time.Time) model.Session {
	t.Helper()
	session := model.Session{
		OrgID:           fx.org.ID,
		ChannelID:       channelID,
		AgentID:         fx.agent.ID,
		CreatedBy:       &createdBy,
		Model:           "deepseek-v4-flash",
		AccessMode:      "full",
		ReasoningEffort: "high",
		Source:          "web",
		Name:            "session-" + uuid.NewString()[:8],
		Status:          "active",
		CreatedAt:       activityAt.Add(-time.Minute),
		UpdatedAt:       activityAt.Add(-time.Minute),
	}
	if err := h.db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	for _, userID := range participantIDs {
		participant := model.SessionParticipant{
			SessionID: session.ID,
			UserID:    userID,
			Role:      "collaborator",
			CreatedAt: activityAt.Add(-time.Minute),
		}
		if err := h.db.Create(&participant).Error; err != nil {
			t.Fatalf("create session participant: %v", err)
		}
	}
	event := model.SessionEvent{
		OrgID:            fx.org.ID,
		SessionID:        session.ID,
		AgentID:          fx.agent.ID,
		RuntimeSessionID: session.ID.String(),
		EventID:          "event-" + uuid.NewString(),
		EventType:        "user.message",
		ActorUserID:      &createdBy,
		Source:           "web",
		SequenceNumber:   1,
		Payload:          model.JSON{"text": session.Name},
		EventAt:          activityAt,
		CreatedAt:        activityAt,
	}
	if err := h.db.Create(&event).Error; err != nil {
		t.Fatalf("create session event: %v", err)
	}
	return session
}
