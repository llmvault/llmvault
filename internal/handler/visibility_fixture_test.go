package handler_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// visFixture is a shared channel/agent visibility fixture: the member belongs to
// teamA (so the teamA channel + its agent + its sessions are visible) but not
// teamB (so the teamB channel + its agent + its sessions are hidden). Admin and
// API-key callers see everything.
type visFixture struct {
	org          model.Org
	admin        model.User
	member       model.User
	visibleAgent model.Agent   // assigned to the teamA (visible) channel
	hiddenAgent  model.Agent   // assigned to the teamB (hidden) channel
	visibleCh    model.Channel // teamA
	hiddenCh     model.Channel // teamB
	visibleSess  uuid.UUID
	hiddenSess   uuid.UUID
}

func seedVisFixture(t *testing.T, db *gorm.DB) visFixture {
	t.Helper()
	org := model.Org{ID: uuid.New(), Name: "vis-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	admin := model.User{ID: uuid.New(), Email: "vadmin-" + uuid.NewString()[:8] + "@test.com", Name: "admin"}
	member := model.User{ID: uuid.New(), Email: "vmember-" + uuid.NewString()[:8] + "@test.com", Name: "member"}

	teamA := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-a-" + uuid.NewString()[:8]}
	teamB := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "team-b-" + uuid.NewString()[:8]}

	mkAgent := func(name string, team uuid.UUID) model.Agent {
		return model.Agent{ID: uuid.New(), OrgID: &org.ID, Name: name, Model: "test", Status: "active", TeamID: team}
	}
	// Agent visibility is team-membership based: the visible agent belongs to the
	// member's team (teamA), the hidden agent to teamB.
	visibleAgent := mkAgent("Visible", teamA.ID)
	hiddenAgent := mkAgent("Hidden", teamB.ID)

	visibleCh := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "vis-ch-" + uuid.NewString()[:8], Kind: "standard", TeamID: &teamA.ID, DefaultAgentID: visibleAgent.ID}
	hiddenCh := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "hid-ch-" + uuid.NewString()[:8], Kind: "standard", TeamID: &teamB.ID, DefaultAgentID: hiddenAgent.ID}

	visibleSess := model.Session{ID: uuid.New(), OrgID: org.ID, ChannelID: visibleCh.ID, AgentID: visibleAgent.ID, Status: "active"}
	hiddenSess := model.Session{ID: uuid.New(), OrgID: org.ID, ChannelID: hiddenCh.ID, AgentID: hiddenAgent.ID, Status: "active"}

	rows := []any{
		&org, &admin, &member,
		&model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "admin"},
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
		&teamA, &teamB,
		&visibleAgent, &hiddenAgent,
		&model.TeamMember{OrgID: org.ID, TeamID: teamA.ID, UserID: member.ID, Role: "member"},
		&visibleCh, &hiddenCh,
		&visibleSess, &hiddenSess,
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed vis fixture: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.Session{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{admin.ID, member.ID}).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	return visFixture{
		org: org, admin: admin, member: member,
		visibleAgent: visibleAgent, hiddenAgent: hiddenAgent,
		visibleCh: visibleCh, hiddenCh: hiddenCh,
		visibleSess: visibleSess.ID, hiddenSess: hiddenSess.ID,
	}
}

// caller identifies a request principal for the visibility tests.
type caller struct {
	user   *model.User
	apiKey bool
}

func memberCaller(fx visFixture) caller { return caller{user: &fx.member} }
func adminCaller(fx visFixture) caller  { return caller{user: &fx.admin} }
func apiKeyCaller() caller              { return caller{apiKey: true} }

func (c caller) apply(req *http.Request, org model.Org) *http.Request {
	req = middleware.WithOrg(req, &org)
	if c.apiKey {
		return middleware.WithAPIKeyClaims(req, &middleware.APIKeyClaims{OrgID: org.ID.String()})
	}
	if c.user != nil {
		req = middleware.WithUser(req, c.user)
		req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: c.user.ID.String(), OrgID: org.ID.String()})
	}
	return req
}

func withURLParam(req *http.Request, key, value string) *http.Request {
	rctx := chi.NewRouteContext()
	rctx.URLParams.Add(key, value)
	return req.WithContext(context.WithValue(req.Context(), chi.RouteCtxKey, rctx))
}
