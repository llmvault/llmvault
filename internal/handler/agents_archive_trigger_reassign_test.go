package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Archiving an agent must not strand its triggers: both the automations list and
// the dispatcher inner-join on non-archived agents, so a trigger left pointing at
// the archived agent silently vanishes from the UI and stops firing while still
// reading enabled=true. The archive re-homes triggers onto the default agent and
// disables them instead.
func TestArchiveAgentReassignsTriggersToDefaultAndDisables(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)

	def := createArchiveTestAgent(t, db, org.ID, "Hivy", true)
	victim := createArchiveTestAgent(t, db, org.ID, "Hakaree", false)
	trigger := createArchiveTestTrigger(t, db, org.ID, victim.ID)

	h := newAgentHandlerForTest(db)
	rr := archiveAgentRequest(t, h, &org, victim.ID)
	if rr.Code != http.StatusOK {
		t.Fatalf("archive status = %d, body = %s", rr.Code, rr.Body.String())
	}

	got := reloadTrigger(t, db, trigger.ID)
	if got.AgentID != def.ID {
		t.Fatalf("trigger agent_id = %s, want reassigned to default %s", got.AgentID, def.ID)
	}
	if got.Enabled {
		t.Fatalf("trigger still enabled after archive; want disabled")
	}
}

// With no default agent to receive them, triggers are disabled in place so they
// at least stop firing (rather than being reassigned to nothing).
func TestArchiveAgentWithoutDefaultDisablesTriggersInPlace(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)

	victim := createArchiveTestAgent(t, db, org.ID, "Hakaree", false)
	trigger := createArchiveTestTrigger(t, db, org.ID, victim.ID)

	h := newAgentHandlerForTest(db)
	rr := archiveAgentRequest(t, h, &org, victim.ID)
	if rr.Code != http.StatusOK {
		t.Fatalf("archive status = %d, body = %s", rr.Code, rr.Body.String())
	}

	got := reloadTrigger(t, db, trigger.ID)
	if got.AgentID != victim.ID {
		t.Fatalf("trigger agent_id = %s, want unchanged %s (no default to reassign to)", got.AgentID, victim.ID)
	}
	if got.Enabled {
		t.Fatalf("trigger still enabled after archive; want disabled in place")
	}
}

// The default Hivy agent cannot be deleted (409), and the archive is a no-op.
func TestArchiveDefaultHivyAgentRejected(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)

	def := createArchiveTestAgent(t, db, org.ID, "Hivy", true)

	h := newAgentHandlerForTest(db)
	rr := archiveAgentRequest(t, h, &org, def.ID)
	if rr.Code != http.StatusConflict {
		t.Fatalf("archive default agent status = %d, want 409; body = %s", rr.Code, rr.Body.String())
	}

	var reloaded model.Agent
	if err := db.First(&reloaded, "id = ?", def.ID).Error; err != nil {
		t.Fatalf("reload agent: %v", err)
	}
	if reloaded.Status == "archived" {
		t.Fatalf("default agent was archived despite guard")
	}
}

func createArchiveTestAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, name string, isDefault bool) model.Agent {
	t.Helper()
	agent := model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		Name:          name,
		IsDefault:     isDefault,
		Model:         "deepseek-v4-flash",
		Tools:         model.JSON{},
		McpServers:    model.RawJSON("[]"),
		Skills:        model.JSON{},
		RuntimeConfig: model.JSON{},
		Permissions:   model.JSON{},
		Resources:     model.JSON{},
		Status:        "active",
	}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent %s: %v", name, err)
	}
	t.Cleanup(func() { db.Where("id = ?", agent.ID).Delete(&model.Agent{}) })
	return agent
}

func createArchiveTestTrigger(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID) model.AgentTrigger {
	t.Helper()
	trigger := model.AgentTrigger{
		OrgID:       orgID,
		AgentID:     agentID,
		TriggerType: "webhook",
		TriggerKey:  "reaction_added",
		Enabled:     true,
	}
	if err := db.Create(&trigger).Error; err != nil {
		t.Fatalf("create trigger: %v", err)
	}
	t.Cleanup(func() { db.Where("id = ?", trigger.ID).Delete(&model.AgentTrigger{}) })
	return trigger
}

func reloadTrigger(t *testing.T, db *gorm.DB, id uuid.UUID) model.AgentTrigger {
	t.Helper()
	var got model.AgentTrigger
	if err := db.First(&got, "id = ?", id).Error; err != nil {
		t.Fatalf("reload trigger: %v", err)
	}
	return got
}

func archiveAgentRequest(t *testing.T, h *handler.AgentHandler, org *model.Org, agentID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodDelete, "/v1/agents/"+agentID.String(), nil)
	req = withChiURLParam(req, "id", agentID.String())
	req = middleware.WithOrg(req, org)
	rr := httptest.NewRecorder()
	h.Archive(rr, req)
	return rr
}
