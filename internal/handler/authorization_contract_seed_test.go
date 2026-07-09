package handler_test

import (
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

// authzWorld is the single-org fixture backing the cross-cutting authorization
// contract suite. It seeds every principal on the role/team lattice the model
// distinguishes — org owner O, org admin A, member M1 (team T1 only), member M2
// (team T2 only) — plus the team resources, org-catalog items, and team
// provisioning grants the matrix probes. Each contract test seeds its own world
// so mutating assertions (role changes, ownership transfer, org delete) never
// leak across test functions.
type authzWorld struct {
	orgDB *gorm.DB
	org   model.Org

	owner model.User
	admin model.User
	m1    model.User // member, team T1
	m2    model.User // member, team T2

	t1 model.Team
	t2 model.Team

	chT1     model.Channel // standard, team T1 (default agent agentT1)
	chT2     model.Channel // standard, team T2 (default agent agentT2)
	chT1Priv model.Channel // private, team T1; M1 is an explicit channel member

	agentT1  model.Agent // team T1
	agentT1b model.Agent // team T1, NOT assigned to any channel (assignable)
	agentT2  model.Agent // team T2

	pluginGranted   model.Plugin // org-installed AND enabled for T1
	pluginUngranted model.Plugin // org-installed, NOT enabled for T1

	srcUngranted ragmodel.RAGSource // org source, not yet granted to any team

	sbT1 model.Sandbox // agentT1 -> visible to M1
	sbT2 model.Sandbox // agentT2 -> hidden from M1

	integration model.Integration
	conn        model.Connection // this org's connection
}

// callers on the authz lattice, reusing the shared caller type.
func (w authzWorld) callerO() caller  { return caller{user: &w.owner} }
func (w authzWorld) callerA() caller  { return caller{user: &w.admin} }
func (w authzWorld) callerM1() caller { return caller{user: &w.m1} }
func (w authzWorld) callerM2() caller { return caller{user: &w.m2} }

func seedAuthzWorld(t *testing.T, db *gorm.DB) authzWorld {
	t.Helper()
	seedDefaultModelCredential(t, db)

	org := model.Org{ID: uuid.New(), Name: "authz-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	owner := model.User{ID: uuid.New(), Email: "o-" + uuid.NewString()[:8] + "@authz.test", Name: "owner"}
	admin := model.User{ID: uuid.New(), Email: "a-" + uuid.NewString()[:8] + "@authz.test", Name: "admin"}
	m1 := model.User{ID: uuid.New(), Email: "m1-" + uuid.NewString()[:8] + "@authz.test", Name: "m1"}
	m2 := model.User{ID: uuid.New(), Email: "m2-" + uuid.NewString()[:8] + "@authz.test", Name: "m2"}

	t1 := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "T1-" + uuid.NewString()[:8]}
	t2 := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "T2-" + uuid.NewString()[:8]}

	mkAgent := func(team uuid.UUID, name string) model.Agent {
		return model.Agent{
			ID: uuid.New(), OrgID: &org.ID, TeamID: team, Name: name + "-" + uuid.NewString()[:8],
			Model: "deepseek-v4-flash", Tools: model.JSON{}, McpServers: model.RawJSON("[]"),
			Skills: model.JSON{}, RuntimeConfig: model.JSON{}, Permissions: model.JSON{},
			Resources: model.JSON{}, Status: "active",
		}
	}
	agentT1 := mkAgent(t1.ID, "agentT1")
	agentT1b := mkAgent(t1.ID, "agentT1b")
	agentT2 := mkAgent(t2.ID, "agentT2")
	hivyT1 := mkAgent(t1.ID, "hivy")
	hivyT1.IsDefault = true
	hivyT2 := mkAgent(t2.ID, "hivy")
	hivyT2.IsDefault = true

	chT1 := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "chT1-" + uuid.NewString()[:8], Kind: "standard", TeamID: &t1.ID, DefaultAgentID: agentT1.ID}
	chT2 := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "chT2-" + uuid.NewString()[:8], Kind: "standard", TeamID: &t2.ID, DefaultAgentID: agentT2.ID}
	chT1Priv := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "priv-" + uuid.NewString()[:8], Kind: "standard", TeamID: &t1.ID, Visibility: "private", DefaultAgentID: agentT1.ID}

	pluginGranted := model.Plugin{ID: uuid.New(), Slug: "authz-granted-" + uuid.NewString()[:8], Name: "Granted", Status: model.PluginStatusActive}
	pluginUngranted := model.Plugin{ID: uuid.New(), Slug: "authz-ungranted-" + uuid.NewString()[:8], Name: "Ungranted", Status: model.PluginStatusActive}

	src := ragmodel.RAGSource{OrgIDValue: org.ID, KindValue: ragmodel.RAGSourceKindWebsite, Name: "authz-src-" + uuid.NewString()[:8], Status: ragmodel.RAGSourceStatusActive, Enabled: true, ConfigValue: model.JSON{"url": "https://authz.example"}}

	integ := model.Integration{ID: uuid.New(), UniqueKey: "authz-uk-" + uuid.NewString()[:8], Provider: "github", DisplayName: "GitHub"}
	conn := model.Connection{ID: uuid.New(), OrgID: org.ID, UserID: admin.ID, IntegrationID: integ.ID, NangoConnectionID: "authz-ncid-" + uuid.NewString()[:8]}

	rows := []any{
		&org, &owner, &admin, &m1, &m2,
		&model.OrgMembership{UserID: owner.ID, OrgID: org.ID, Role: "owner"},
		&model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "admin"},
		&model.OrgMembership{UserID: m1.ID, OrgID: org.ID, Role: "member"},
		&model.OrgMembership{UserID: m2.ID, OrgID: org.ID, Role: "member"},
		&t1, &t2,
		&model.TeamMember{OrgID: org.ID, TeamID: t1.ID, UserID: m1.ID, Role: "member"},
		&model.TeamMember{OrgID: org.ID, TeamID: t2.ID, UserID: m2.ID, Role: "member"},
		&agentT1, &agentT1b, &agentT2, &hivyT1, &hivyT2,
		&chT1, &chT2, &chT1Priv,
		&model.ChannelMember{ChannelID: chT1Priv.ID, UserID: m1.ID, Role: "member"},
		&pluginGranted, &pluginUngranted,
		&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: pluginGranted.ID},
		&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: pluginUngranted.ID},
		&model.TeamPlugin{OrgID: org.ID, TeamID: t1.ID, PluginID: pluginGranted.ID},
		&src,
		&integ, &conn,
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed authz row %T: %v", r, err)
		}
	}

	sbT1 := seedVisSandbox(t, db, org.ID, &agentT1.ID, "authz-sbT1-"+uuid.NewString()[:6])
	sbT2 := seedVisSandbox(t, db, org.ID, &agentT2.ID, "authz-sbT2-"+uuid.NewString()[:6])

	t.Cleanup(func() {
		db.Where("channel_id IN (?)", db.Model(&model.Channel{}).Select("id").Where("org_id = ?", org.ID)).Delete(&model.ChannelMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.AgentTrigger{})
		db.Where("org_id = ?", org.ID).Delete(&model.AgentSchedule{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamRagSource{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("id IN ?", []uuid.UUID{pluginGranted.ID, pluginUngranted.ID}).Delete(&model.Plugin{})
		db.Where("id = ?", src.ID).Delete(&ragmodel.RAGSource{})
		db.Where("id = ?", conn.ID).Delete(&model.Connection{})
		db.Where("id = ?", integ.ID).Delete(&model.Integration{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamMember{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{owner.ID, admin.ID, m1.ID, m2.ID}).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	return authzWorld{
		orgDB: db, org: org, owner: owner, admin: admin, m1: m1, m2: m2,
		t1: t1, t2: t2, chT1: chT1, chT2: chT2, chT1Priv: chT1Priv,
		agentT1: agentT1, agentT1b: agentT1b, agentT2: agentT2,
		pluginGranted: pluginGranted, pluginUngranted: pluginUngranted, srcUngranted: src,
		sbT1: sbT1, sbT2: sbT2, integration: integ, conn: conn,
	}
}
