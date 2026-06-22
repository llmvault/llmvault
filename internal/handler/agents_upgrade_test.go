package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestStartSandboxUpgradeRejectsPerSessionAgent(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := createAgentModelUpdateTestAgent(t, db, org.ID, "per_session")
	enq := &enqueue.MockClient{}
	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())
	h.SetEnqueuer(enq)

	rr := startSandboxUpgrade(t, h, &org, agent.ID)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "per-session agents do not use sandbox upgrades" {
		t.Fatalf("error=%q", body["error"])
	}
	if len(enq.Tasks()) != 0 {
		t.Fatalf("enqueued %d tasks, want 0", len(enq.Tasks()))
	}
	var upgrades int64
	if err := db.Model(&model.AgentSandboxUpgrade{}).Where("agent_id = ?", agent.ID).Count(&upgrades).Error; err != nil {
		t.Fatalf("count upgrades: %v", err)
	}
	if upgrades != 0 {
		t.Fatalf("upgrade rows=%d, want 0", upgrades)
	}
}

func TestStartSandboxUpgradeRejectsPerSessionAgentWithExistingUpgrade(t *testing.T) {
	db := connectTestDB(t)
	org := createTestOrg(t, db)
	agent := createAgentModelUpdateTestAgent(t, db, org.ID, "per_session")
	existing := model.AgentSandboxUpgrade{
		OrgID:   org.ID,
		AgentID: agent.ID,
		Status:  model.AgentSandboxUpgradeStatusQueued,
		Phase:   model.AgentSandboxUpgradePhaseQueued,
	}
	if err := db.Create(&existing).Error; err != nil {
		t.Fatalf("create existing upgrade: %v", err)
	}
	enq := &enqueue.MockClient{}
	h := handler.NewAgentHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())
	h.SetEnqueuer(enq)

	rr := startSandboxUpgrade(t, h, &org, agent.ID)
	if rr.Code != http.StatusConflict {
		t.Fatalf("status=%d body=%s", rr.Code, rr.Body.String())
	}
	var body map[string]string
	if err := json.NewDecoder(rr.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body["error"] != "per-session agents do not use sandbox upgrades" {
		t.Fatalf("error=%q", body["error"])
	}
	if len(enq.Tasks()) != 0 {
		t.Fatalf("enqueued %d tasks, want 0", len(enq.Tasks()))
	}
}

func startSandboxUpgrade(t *testing.T, h *handler.AgentHandler, org *model.Org, agentID uuid.UUID) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agentID.String()+"/sandbox/upgrade", nil)
	req = withChiURLParam(req, "id", agentID.String())
	req = middleware.WithOrg(req, org)
	rr := httptest.NewRecorder()
	h.StartSandboxUpgrade(rr, req)
	return rr
}
