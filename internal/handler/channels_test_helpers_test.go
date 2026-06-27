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

func seedChannelConnection(t *testing.T, h *channelHarness, fx channelFixture, provider, nangoID string) model.Connection {
	t.Helper()
	integ := createTestIntegration(t, h.db, provider)
	conn := model.Connection{
		OrgID:             fx.org.ID,
		UserID:            fx.owner.ID,
		IntegrationID:     integ.ID,
		NangoConnectionID: nangoID,
		Meta:              model.JSON{},
	}
	if err := h.db.Create(&conn).Error; err != nil {
		t.Fatalf("create %s connection: %v", provider, err)
	}
	return conn
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

func assertChannelNameSet(t *testing.T, rr *httptest.ResponseRecorder, want []string) {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("list status=%d body=%s", rr.Code, rr.Body.String())
	}
	var out channelListOut
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode list: %v\n%s", err, rr.Body.String())
	}
	got := map[string]bool{}
	for _, channel := range out.Data {
		got[channel.Name] = true
	}
	if len(got) != len(want) {
		t.Fatalf("channel names=%v, want=%v", got, want)
	}
	for _, name := range want {
		if !got[name] {
			t.Fatalf("channel names=%v, want %q", got, name)
		}
	}
}

func seedChannelTeam(t *testing.T, h *channelHarness, fx channelFixture, name string) model.Team {
	t.Helper()
	team := model.Team{OrgID: fx.org.ID, Name: name, Description: name + " team", CreatedBy: &fx.owner.ID}
	if err := h.db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	return team
}

func createTeamChannelForTest(t *testing.T, h *channelHarness, fx channelFixture, name string, teamID uuid.UUID) string {
	t.Helper()
	rr := h.doJSON(t, http.MethodPost, "/v1/channels", fx, fx.owner, map[string]any{
		"name":             name,
		"team_id":          teamID.String(),
		"default_agent_id": fx.agent.ID.String(),
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("create team channel %s status=%d body=%s", name, rr.Code, rr.Body.String())
	}
	out := decodeChannelCreate(t, rr)
	if out.Channel.TeamID == nil || *out.Channel.TeamID != teamID.String() {
		t.Fatalf("team_id=%v, want %s", out.Channel.TeamID, teamID)
	}
	return out.Channel.ID
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
