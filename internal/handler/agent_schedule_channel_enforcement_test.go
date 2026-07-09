package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// scheduleAuthzFixture exercises the POST /v1/schedules authorization gate. The
// agent lives in teamA, whose default channel is where an omitted channel_id
// resolves to; the outsider is an org member on teamB only.
type scheduleAuthzFixture struct {
	org            model.Org
	owner          model.User // org owner => manager
	teamMember     model.User // org member, active on teamA
	outsider       model.User // org member, on teamB only (not agent's team)
	teamA          model.Team
	teamB          model.Team
	agent          model.Agent
	defaultChannel model.Channel
	router         *chi.Mux
	db             *gorm.DB
}

func newScheduleAuthzHarness(t *testing.T) scheduleAuthzFixture {
	t.Helper()
	db := connectTestDB(t)
	org := createTestOrg(t, db)

	owner := seedOrgUser(t, db, org.ID, "owner")
	teamMember := seedOrgUser(t, db, org.ID, "member")
	outsider := seedOrgUser(t, db, org.ID, "member")
	teamA := seedTeam(t, db, org.ID, "sched-team-a")
	teamB := seedTeam(t, db, org.ID, "sched-team-b")
	seedTeamMember(t, db, org.ID, teamA.ID, teamMember.ID)
	seedTeamMember(t, db, org.ID, teamB.ID, outsider.ID)

	agent := seedTeamAgent(t, db, org.ID, teamA.ID)
	defaultChannel := seedTeamDefaultChannel(t, db, org.ID, teamA.ID, agent.ID, owner.ID)

	h := handler.NewScheduleHandler(db)
	r := chi.NewRouter()
	r.Post("/v1/schedules", h.Create)

	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.AgentSchedule{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
	})
	return scheduleAuthzFixture{
		org:            org,
		owner:          owner,
		teamMember:     teamMember,
		outsider:       outsider,
		teamA:          teamA,
		teamB:          teamB,
		agent:          agent,
		defaultChannel: defaultChannel,
		router:         r,
		db:             db,
	}
}

// seedTeamDefaultChannel creates the team's Hivy-anchored default channel, the
// team-scoped #general that an omitted schedule channel_id resolves to.
func seedTeamDefaultChannel(t *testing.T, db *gorm.DB, orgID, teamID, agentID, createdBy uuid.UUID) model.Channel {
	t.Helper()
	channel := model.Channel{
		OrgID:          orgID,
		TeamID:         teamID,
		Name:           "general-" + uuid.NewString()[:8],
		Kind:           "standard",
		Visibility:     "public",
		DefaultAgentID: agentID,
		IsDefault:      true,
		Origin:         "native",
		CreatedBy:      &createdBy,
	}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create team default channel: %v", err)
	}
	return channel
}

func (fx scheduleAuthzFixture) createSchedule(t *testing.T, user *model.User, body map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	_ = json.NewEncoder(&buf).Encode(body)
	req := httptest.NewRequest(http.MethodPost, "/v1/schedules", &buf)
	req.Header.Set("Content-Type", "application/json")
	org := fx.org
	req = middleware.WithOrg(req, &org)
	if user != nil {
		u := *user
		req = middleware.WithUser(req, &u)
		req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: user.ID.String(), OrgID: fx.org.ID.String()})
	}
	rr := httptest.NewRecorder()
	fx.router.ServeHTTP(rr, req)
	return rr
}

func (fx scheduleAuthzFixture) scheduleCount(t *testing.T) int64 {
	t.Helper()
	var n int64
	if err := fx.db.Model(&model.AgentSchedule{}).Where("org_id = ?", fx.org.ID).Count(&n).Error; err != nil {
		t.Fatalf("count schedules: %v", err)
	}
	return n
}

// TestScheduleCreate_OutsiderNoChannelDenied is the regression for the bypass:
// an org member who is NOT in the agent's team omits channel_id, and must be
// refused (was silently bound to the agent's team #general and fired).
func TestScheduleCreate_OutsiderNoChannelDenied(t *testing.T) {
	fx := newScheduleAuthzHarness(t)
	rr := fx.createSchedule(t, &fx.outsider, map[string]any{
		"name":            "sneaky cron",
		"agent_id":        fx.agent.ID.String(),
		"task_prompt":     "exfiltrate secrets",
		"cron_expression": "0 9 * * *",
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403; body=%s", rr.Code, rr.Body.String())
	}
	if n := fx.scheduleCount(t); n != 0 {
		t.Fatalf("schedule count=%d, want 0 (nothing should be created)", n)
	}
}

// TestScheduleCreate_TeamMemberNoChannelBindsDefault confirms that a member of
// the agent's team may omit channel_id and the schedule binds to that team's
// default channel.
func TestScheduleCreate_TeamMemberNoChannelBindsDefault(t *testing.T) {
	fx := newScheduleAuthzHarness(t)
	rr := fx.createSchedule(t, &fx.teamMember, map[string]any{
		"name":            "team-member cron",
		"agent_id":        fx.agent.ID.String(),
		"task_prompt":     "daily digest",
		"cron_expression": "0 9 * * *",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Schedule struct {
			ID        string `json:"id"`
			ChannelID string `json:"channel_id"`
		} `json:"schedule"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if out.Schedule.ChannelID != fx.defaultChannel.ID.String() {
		t.Fatalf("channel_id=%s, want team default %s", out.Schedule.ChannelID, fx.defaultChannel.ID)
	}
}

// TestScheduleCreate_ManagerNoChannelBindsDefault confirms an org manager (not a
// team member) may also omit channel_id and bind to the team default.
func TestScheduleCreate_ManagerNoChannelBindsDefault(t *testing.T) {
	fx := newScheduleAuthzHarness(t)
	rr := fx.createSchedule(t, &fx.owner, map[string]any{
		"name":            "manager cron",
		"agent_id":        fx.agent.ID.String(),
		"task_prompt":     "manager digest",
		"cron_expression": "0 9 * * *",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Schedule struct {
			ChannelID string `json:"channel_id"`
		} `json:"schedule"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if out.Schedule.ChannelID != fx.defaultChannel.ID.String() {
		t.Fatalf("channel_id=%s, want team default %s", out.Schedule.ChannelID, fx.defaultChannel.ID)
	}
}

// TestScheduleCreate_ExplicitChannelUnchanged confirms the explicit channel_id
// path still works for an authorized team member and binds to that channel.
func TestScheduleCreate_ExplicitChannelUnchanged(t *testing.T) {
	fx := newScheduleAuthzHarness(t)
	rr := fx.createSchedule(t, &fx.teamMember, map[string]any{
		"name":            "explicit cron",
		"agent_id":        fx.agent.ID.String(),
		"channel_id":      fx.defaultChannel.ID.String(),
		"task_prompt":     "explicit digest",
		"cron_expression": "0 9 * * *",
	})
	if rr.Code != http.StatusCreated {
		t.Fatalf("status=%d, want 201; body=%s", rr.Code, rr.Body.String())
	}
	var out struct {
		Schedule struct {
			ChannelID string `json:"channel_id"`
		} `json:"schedule"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &out); err != nil {
		t.Fatalf("decode: %v; body=%s", err, rr.Body.String())
	}
	if out.Schedule.ChannelID != fx.defaultChannel.ID.String() {
		t.Fatalf("channel_id=%s, want %s", out.Schedule.ChannelID, fx.defaultChannel.ID)
	}
}

// TestScheduleCreate_ExplicitForeignChannelDenied confirms an outsider supplying
// an explicit channel_id they cannot use is still denied (path already correct).
func TestScheduleCreate_ExplicitForeignChannelDenied(t *testing.T) {
	fx := newScheduleAuthzHarness(t)
	rr := fx.createSchedule(t, &fx.outsider, map[string]any{
		"name":            "explicit foreign cron",
		"agent_id":        fx.agent.ID.String(),
		"channel_id":      fx.defaultChannel.ID.String(),
		"task_prompt":     "explicit foreign digest",
		"cron_expression": "0 9 * * *",
	})
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status=%d, want 403; body=%s", rr.Code, rr.Body.String())
	}
	if n := fx.scheduleCount(t); n != 0 {
		t.Fatalf("schedule count=%d, want 0", n)
	}
}
