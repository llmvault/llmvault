package handler_test

import (
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

// contractWorld is the single seeded org that backs the cross-cutting visibility
// contract test. It builds on the shared seedVisFixture (member on teamA, admin,
// agentA->chanA visible, agentB->chanB hidden, sessA/sessB) and layers a full
// B-world of resources the member must never see, plus an A-world positive
// control the member SHOULD see. forbidden lists every token whose presence in a
// member-facing response body is a leak.
type contractWorld struct {
	base visFixture

	// B-world ids used to build by-id URLs (must 403/404 for the member).
	triggerB  uuid.UUID
	schedB    uuid.UUID
	sandboxB  uuid.UUID
	artifactB uuid.UUID
	deliveryB uuid.UUID

	// A-world positive-control tokens (must appear for the member).
	triggerA   uuid.UUID
	sandboxA   uuid.UUID
	projA      uuid.UUID
	artifactA  uuid.UUID
	schedATask string
	srcA       uuid.UUID
	srcAName   string
	memA       string
	obsA       string
	dirA       string
	sessOwnedA uuid.UUID

	// shared, org-wide handles.
	projB       uuid.UUID
	srcB        uuid.UUID
	pluginSlug  string
	catalogSlug string

	// admin anti-vacuous tokens (admin must still see the B-world resource).
	agentBInstr   string
	triggerBInstr string
	schedBTask    string
	deliveryBMark string
	memB          string
	obsB          string
	dirB          string

	forbidden []string
}

func mark(label string) string { return "VISZ-" + label + "-" + uuid.NewString() }

func seedContractWorld(t *testing.T, db *gorm.DB) contractWorld {
	t.Helper()
	fx := seedVisFixture(t, db)
	org := fx.org
	now := time.Now().UTC()

	w := contractWorld{base: fx}
	w.agentBInstr = mark("AGENTB-INSTR")
	w.triggerBInstr = mark("TRIGGERB-INSTR")
	w.schedATask = mark("SCHEDA-TASK")
	w.schedBTask = mark("SCHEDB-TASK")
	w.deliveryBMark = mark("DELIVERYB")
	w.srcAName = mark("SRCA-NAME")
	w.memA, w.memB = mark("MEMA"), mark("MEMB")
	w.obsA, w.obsB = mark("OBSA"), mark("OBSB")
	w.dirA, w.dirB = mark("DIRA"), mark("DIRB")
	w.pluginSlug = "vis-plugin-" + uuid.NewString()[:8]
	w.catalogSlug = "vis-catalog-" + uuid.NewString()[:8]

	// agentB gets distinctive instructions the member must never read.
	if err := db.Model(&model.Agent{}).Where("id = ?", fx.hiddenAgent.ID).
		Update("instructions", w.agentBInstr).Error; err != nil {
		t.Fatalf("set agentB instructions: %v", err)
	}

	// Extra positive-control channels the member CAN use (external / null team).
	agentShared := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: fx.visibleAgent.TeamID, Name: "shared-" + uuid.NewString()[:8], Model: "test", Status: "active"}
	chanNull := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "null-" + uuid.NewString()[:8], Kind: "standard", DefaultAgentID: agentShared.ID}
	chanExt := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "ext-" + uuid.NewString()[:8], Kind: "standard", Origin: "external", DefaultAgentID: agentShared.ID}
	chanSys := model.Channel{ID: uuid.New(), OrgID: org.ID, Name: "sys-" + uuid.NewString()[:8], Kind: "system", DefaultAgentID: agentShared.ID}

	triggerA := model.AgentTrigger{
		ID: uuid.New(), OrgID: org.ID, AgentID: fx.visibleAgent.ID, TriggerType: "webhook",
		ChannelID: &fx.visibleCh.ID, TriggerKeys: pq.StringArray{"issues.opened"}, TriggerKey: "issues.opened",
		Enabled: true, Instructions: "usable-instructions",
	}
	triggerB := model.AgentTrigger{
		ID: uuid.New(), OrgID: org.ID, AgentID: fx.hiddenAgent.ID, TriggerType: "webhook",
		ChannelID: &fx.hiddenCh.ID, TriggerKeys: pq.StringArray{"issues.opened"}, TriggerKey: "issues.opened",
		Enabled: true, Instructions: w.triggerBInstr,
	}
	schedA := model.AgentSchedule{
		ID: uuid.New(), OrgID: org.ID, AgentID: fx.visibleAgent.ID, RuntimeJobID: "job-" + uuid.NewString(),
		Status: "active", ScheduleKind: "interval", Channel: fx.visibleCh.ID.String(), TaskPrompt: w.schedATask,
	}
	schedB := model.AgentSchedule{
		ID: uuid.New(), OrgID: org.ID, AgentID: fx.hiddenAgent.ID, RuntimeJobID: "job-" + uuid.NewString(),
		Status: "active", ScheduleKind: "interval", Channel: fx.hiddenCh.ID.String(), TaskPrompt: w.schedBTask,
	}
	// The flat session list only surfaces sessions the member owns/participates
	// in, so seed a member-owned session in chanA as the positive control.
	sessOwnedA := model.Session{ID: uuid.New(), OrgID: org.ID, ChannelID: fx.visibleCh.ID, AgentID: fx.visibleAgent.ID, Status: "active", CreatedBy: &fx.member.ID}
	w.sessOwnedA = sessOwnedA.ID
	deliveryB := model.AgentTriggerDelivery{
		ID: uuid.New(), OrgID: org.ID, AgentID: fx.hiddenAgent.ID, TriggerID: triggerB.ID,
		DeliveryID: w.deliveryBMark, SessionID: fx.hiddenSess, Payload: model.RawJSON(`{"marker":"` + w.deliveryBMark + `"}`),
	}

	plugin := model.Plugin{ID: uuid.New(), Slug: w.pluginSlug, Name: "Vis Plugin", Status: model.PluginStatusActive}
	catalog := model.AgentCatalog{
		ID: uuid.New(), Slug: w.catalogSlug, Name: "Vis Catalog " + uuid.NewString()[:8], Model: "test",
		SandboxImage: model.SandboxImageDefault, Manifest: model.RawJSON(`{}`), Status: model.AgentCatalogStatusActive,
	}
	srcA := ragmodel.RAGSource{OrgIDValue: org.ID, KindValue: ragmodel.RAGSourceKindWebsite, Name: w.srcAName, Status: ragmodel.RAGSourceStatusActive, Enabled: true, ConfigValue: model.JSON{"url": "https://a.example"}}
	srcB := ragmodel.RAGSource{OrgIDValue: org.ID, KindValue: ragmodel.RAGSourceKindWebsite, Name: mark("SRCB-NAME"), Status: ragmodel.RAGSourceStatusActive, Enabled: true, ConfigValue: model.JSON{"url": "https://b.example"}}

	rows := []any{
		&agentShared, &chanNull, &chanExt, &chanSys, &sessOwnedA,
		&triggerA, &triggerB, &schedA, &schedB, &deliveryB,
		&plugin, &catalog,
		&model.OrgPluginInstall{ID: uuid.New(), OrgID: org.ID, PluginID: plugin.ID},
		&srcA, &srcB,
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed contract row %T: %v", r, err)
		}
	}
	// enabled_agent_ids is team-derived: grant the plugin to both agents' teams so
	// each agent resolves it, mirroring the removed per-agent installs' intent.
	grantPluginToTeam(t, db, org.ID, fx.visibleAgent.TeamID, plugin.ID)
	grantPluginToTeam(t, db, org.ID, fx.hiddenAgent.TeamID, plugin.ID)
	// Knowledge grants are team-derived: grant srcA to teamA (visibleCh's team, so
	// the member sees it) and srcB to teamB (hiddenCh's team, so the member does
	// not). Mirrors the removed per-channel grants' intent.
	if err := db.Create(&model.TeamRagSource{OrgID: org.ID, TeamID: *fx.visibleCh.TeamID, RagSourceID: srcA.ID}).Error; err != nil {
		t.Fatalf("grant srcA: %v", err)
	}
	if err := db.Create(&model.TeamRagSource{OrgID: org.ID, TeamID: *fx.hiddenCh.TeamID, RagSourceID: srcB.ID}).Error; err != nil {
		t.Fatalf("grant srcB: %v", err)
	}
	// The installed agent for this catalog is only on chanB, so the member must
	// not learn its id via installed_agent_id.
	if err := db.Model(&model.Agent{}).Where("id = ?", fx.hiddenAgent.ID).
		Update("agent_catalog_id", catalog.ID).Error; err != nil {
		t.Fatalf("link agentB to catalog: %v", err)
	}

	// Sandboxes, canvas, memories, observations, directives via shared helpers.
	w.sandboxA = seedVisSandbox(t, db, org.ID, &fx.visibleAgent.ID, "vis-sb-"+uuid.NewString()[:8]).ID
	w.sandboxB = seedVisSandbox(t, db, org.ID, &fx.hiddenAgent.ID, "hid-sb-"+uuid.NewString()[:8]).ID
	projA := seedCanvasArtifactProject(t, db, org.ID, "vproj-"+uuid.NewString()[:8], "Visible proj", now)
	projB := seedCanvasArtifactProject(t, db, org.ID, "hproj-"+uuid.NewString()[:8], "Hidden proj", now.Add(time.Minute))
	w.projA = projA.ID
	w.artifactA = seedCanvasArtifact(t, db, org.ID, projA.ID, "vis-art-"+uuid.NewString()[:8], now.Add(2*time.Minute), &fx.visibleSess).ID
	w.artifactB = seedCanvasArtifact(t, db, org.ID, projB.ID, "hid-art-"+uuid.NewString()[:8], now.Add(3*time.Minute), &fx.hiddenSess).ID
	seedMemoryRow(t, db, org.ID, &fx.visibleCh.ID, w.memA)
	seedMemoryRow(t, db, org.ID, &fx.hiddenCh.ID, w.memB)
	seedObservation(t, db, org.ID, &fx.visibleCh.ID, w.obsA, 3)
	seedObservation(t, db, org.ID, &fx.hiddenCh.ID, w.obsB, 3)
	seedDirectiveRow(t, db, org.ID, &fx.visibleCh.ID, w.dirA)
	seedDirectiveRow(t, db, org.ID, &fx.hiddenCh.ID, w.dirB)

	w.triggerA, w.triggerB = triggerA.ID, triggerB.ID
	w.schedB, w.deliveryB = schedB.ID, deliveryB.ID
	w.srcA, w.srcB = srcA.ID, srcB.ID
	w.projB = projB.ID

	w.forbidden = []string{
		fx.hiddenAgent.ID.String(), fx.hiddenCh.ID.String(), fx.hiddenSess.String(),
		triggerB.ID.String(), schedB.ID.String(), deliveryB.ID.String(),
		w.sandboxB.String(), w.artifactB.String(), projB.ID.String(),
		w.agentBInstr, w.triggerBInstr, w.schedBTask, w.deliveryBMark,
		w.memB, w.obsB, w.dirB,
	}

	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.CanvasArtifact{})
		db.Where("org_id = ?", org.ID).Delete(&model.CanvasProject{})
		db.Where("org_id = ?", org.ID).Delete(&model.AgentTriggerDelivery{})
		db.Where("org_id = ?", org.ID).Delete(&model.AgentSchedule{})
		db.Where("org_id = ?", org.ID).Delete(&model.AgentTrigger{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamPlugin{})
		db.Where("org_id = ?", org.ID).Delete(&model.OrgPluginInstall{})
		db.Where("id = ?", plugin.ID).Delete(&model.Plugin{})
		db.Where("id = ?", catalog.ID).Delete(&model.AgentCatalog{})
		db.Where("org_id = ?", org.ID).Delete(&model.TeamRagSource{})
		db.Where("id IN ?", []uuid.UUID{srcA.ID, srcB.ID}).Delete(&ragmodel.RAGSource{})
	})
	return w
}
