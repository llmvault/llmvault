package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// triggerVisFixture seeds a member who is NOT on a team, an admin, a channel any
// member can use (external), and a team-scoped private channel the member cannot
// use — each with its own agent + trigger. It exercises the read-side channel /
// agent visibility gating for triggers and trigger-deliveries.
type triggerVisFixture struct {
	org          model.Org
	member       model.User
	admin        model.User
	usableChan   model.Channel
	teamChan     model.Channel
	usableTrig   model.AgentTrigger
	hiddenTrig   model.AgentTrigger
	visibleAgent model.Agent
	hiddenAgent  model.Agent
}

const hiddenTriggerInstructions = "SECRET-HIDDEN-INSTRUCTIONS"

func seedTriggerVisFixture(t *testing.T, db *gorm.DB) triggerVisFixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "trig-vis-" + uuid.NewString(), Active: true, RateLimit: 1000}
	// memberTeam owns the visible agent and the member belongs to it; otherTeam
	// owns the hidden agent and the member is not on it. Agent visibility is now
	// team-membership based (channelagents.VisibleAgentIDsSubquery).
	memberTeam := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "mteam-" + uuid.NewString()}
	otherTeam := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "oteam-" + uuid.NewString()}
	visibleAgent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "vis-" + uuid.NewString(), Model: "test", Status: "active", TeamID: memberTeam.ID}
	hiddenAgent := model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: "hid-" + uuid.NewString(), Model: "test", Status: "active", TeamID: otherTeam.ID}
	usableChan := model.Channel{
		ID: uuid.New(), OrgID: org.ID, Name: "usable-" + uuid.NewString(),
		DefaultAgentID: visibleAgent.ID, Origin: "external",
		ExternalProvider: "slack", ExternalResourceKey: "C0" + uuid.NewString()[:8],
	}
	teamChan := model.Channel{
		ID: uuid.New(), OrgID: org.ID, Name: "team-" + uuid.NewString(),
		DefaultAgentID: hiddenAgent.ID, Visibility: "private", TeamID: &otherTeam.ID,
	}
	member := model.User{ID: uuid.New(), Email: "m-" + uuid.NewString() + "@ex.test", Name: "Member"}
	admin := model.User{ID: uuid.New(), Email: "a-" + uuid.NewString() + "@ex.test", Name: "Admin"}
	memberMem := model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"}
	adminMem := model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "admin"}
	memberTeamMem := model.TeamMember{OrgID: org.ID, TeamID: memberTeam.ID, UserID: member.ID, Role: "member"}
	usableTrig := model.AgentTrigger{
		ID: uuid.New(), OrgID: org.ID, AgentID: visibleAgent.ID, TriggerType: "webhook",
		ChannelID: &usableChan.ID, TriggerKeys: pq.StringArray{"issues.opened"}, TriggerKey: "issues.opened",
		Enabled: true, Instructions: "usable instructions",
	}
	hiddenTrig := model.AgentTrigger{
		ID: uuid.New(), OrgID: org.ID, AgentID: hiddenAgent.ID, TriggerType: "webhook",
		ChannelID: &teamChan.ID, TriggerKeys: pq.StringArray{"issues.opened"}, TriggerKey: "issues.opened",
		Enabled: true, Instructions: hiddenTriggerInstructions,
	}

	rows := []any{&org, &memberTeam, &otherTeam, &visibleAgent, &hiddenAgent, &usableChan, &teamChan,
		&member, &admin, &memberMem, &adminMem, &memberTeamMem, &usableTrig, &hiddenTrig}
	for _, row := range rows {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("seed %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.AgentTrigger{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Delete(&model.User{}, "id IN ?", []uuid.UUID{member.ID, admin.ID})
		db.Delete(&model.Org{}, "id = ?", org.ID)
	})
	return triggerVisFixture{
		org: org, member: member, admin: admin, usableChan: usableChan, teamChan: teamChan,
		usableTrig: usableTrig, hiddenTrig: hiddenTrig, visibleAgent: visibleAgent, hiddenAgent: hiddenAgent,
	}
}

func triggerListAs(t *testing.T, db *gorm.DB, fx triggerVisFixture, user *model.User, apiKey bool) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/triggers", nil)
	req = middleware.WithOrg(req, &fx.org)
	if user != nil {
		req = middleware.WithUser(req, user)
	}
	if apiKey {
		req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{OrgID: fx.org.ID.String()})
	}
	rr := httptest.NewRecorder()
	NewTriggerHandler(db).List(rr, req)
	return rr
}

func triggerIDsFromList(t *testing.T, rr *httptest.ResponseRecorder) map[string]bool {
	t.Helper()
	if rr.Code != http.StatusOK {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var resp triggerListResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	ids := map[string]bool{}
	for _, tr := range resp.Data {
		ids[tr.ID] = true
	}
	return ids
}

func TestTriggerListHidesTriggersMemberCannotUse(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	fx := seedTriggerVisFixture(t, db)

	rr := triggerListAs(t, db, fx, &fx.member, false)
	ids := triggerIDsFromList(t, rr)
	if !ids[fx.usableTrig.ID.String()] {
		t.Fatalf("member should see usable trigger; got %v", ids)
	}
	if ids[fx.hiddenTrig.ID.String()] {
		t.Fatalf("member must not see other-team trigger; got %v", ids)
	}
	// Instructions of a hidden trigger must never be serialized to the member.
	if strings.Contains(rr.Body.String(), hiddenTriggerInstructions) {
		t.Fatalf("hidden trigger instructions leaked in list body")
	}
}

func TestTriggerListManagerAndAPIKeySeeAll(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	fx := seedTriggerVisFixture(t, db)

	adminIDs := triggerIDsFromList(t, triggerListAs(t, db, fx, &fx.admin, false))
	if !adminIDs[fx.usableTrig.ID.String()] || !adminIDs[fx.hiddenTrig.ID.String()] {
		t.Fatalf("admin should see all triggers; got %v", adminIDs)
	}
	apiIDs := triggerIDsFromList(t, triggerListAs(t, db, fx, nil, true))
	if !apiIDs[fx.usableTrig.ID.String()] || !apiIDs[fx.hiddenTrig.ID.String()] {
		t.Fatalf("api-key should see all triggers; got %v", apiIDs)
	}
}

func TestTriggerGetHiddenTriggerReturns404(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	fx := seedTriggerVisFixture(t, db)

	getAs := func(triggerID string, user *model.User, apiKey bool) *httptest.ResponseRecorder {
		req := triggerRouteRequest(http.MethodGet, "/v1/triggers/"+triggerID, triggerID, nil)
		req = middleware.WithOrg(req, &fx.org)
		if user != nil {
			req = middleware.WithUser(req, user)
		}
		if apiKey {
			req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{OrgID: fx.org.ID.String()})
		}
		rr := httptest.NewRecorder()
		NewTriggerHandler(db).Get(rr, req)
		return rr
	}

	if rr := getAs(fx.hiddenTrig.ID.String(), &fx.member, false); rr.Code != http.StatusNotFound {
		t.Fatalf("member get hidden trigger: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if strings.Contains(getAs(fx.hiddenTrig.ID.String(), &fx.member, false).Body.String(), hiddenTriggerInstructions) {
		t.Fatalf("hidden trigger instructions leaked on 404")
	}
	if rr := getAs(fx.usableTrig.ID.String(), &fx.member, false); rr.Code != http.StatusOK {
		t.Fatalf("member get usable trigger: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := getAs(fx.hiddenTrig.ID.String(), &fx.admin, false); rr.Code != http.StatusOK {
		t.Fatalf("admin get hidden trigger: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := getAs(fx.hiddenTrig.ID.String(), nil, true); rr.Code != http.StatusOK {
		t.Fatalf("api-key get hidden trigger: status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func TestTriggerDeliveriesGatedByAgentVisibility(t *testing.T) {
	db := connectNangoSlackTestDB(t)
	fx := seedTriggerVisFixture(t, db)

	listAs := func(agentID string, user *model.User, apiKey bool) *httptest.ResponseRecorder {
		req := triggerRouteRequest(http.MethodGet, "/v1/agents/"+agentID+"/trigger-deliveries", agentID, nil)
		req = middleware.WithOrg(req, &fx.org)
		if user != nil {
			req = middleware.WithUser(req, user)
		}
		if apiKey {
			req = middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{OrgID: fx.org.ID.String()})
		}
		rr := httptest.NewRecorder()
		NewTriggerDeliveryHandler(db).List(rr, req)
		return rr
	}

	// Member cannot reach an agent assigned only to a channel they cannot use.
	if rr := listAs(fx.hiddenAgent.ID.String(), &fx.member, false); rr.Code != http.StatusNotFound {
		t.Fatalf("member deliveries for hidden agent: status=%d body=%s", rr.Code, rr.Body.String())
	}
	// Member CAN reach an agent assigned to a channel they can use.
	if rr := listAs(fx.visibleAgent.ID.String(), &fx.member, false); rr.Code != http.StatusOK {
		t.Fatalf("member deliveries for visible agent: status=%d body=%s", rr.Code, rr.Body.String())
	}
	// Admin and API-key reach any agent.
	if rr := listAs(fx.hiddenAgent.ID.String(), &fx.admin, false); rr.Code != http.StatusOK {
		t.Fatalf("admin deliveries for hidden agent: status=%d body=%s", rr.Code, rr.Body.String())
	}
	if rr := listAs(fx.hiddenAgent.ID.String(), nil, true); rr.Code != http.StatusOK {
		t.Fatalf("api-key deliveries for hidden agent: status=%d body=%s", rr.Code, rr.Body.String())
	}
}
