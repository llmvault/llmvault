package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

func (h *agentHarness) postUpgrade(t *testing.T, m orgWithMember, agentID uuid.UUID, body any, role string) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	if body != nil {
		_ = json.NewEncoder(buf).Encode(body)
	}
	req := httptest.NewRequest(http.MethodPost, "/v1/agents/"+agentID.String()+"/sandbox/upgrade", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func (h *agentHarness) getUpgrade(t *testing.T, m orgWithMember, agentID, upgradeID uuid.UUID, role string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest(http.MethodGet, "/v1/agents/"+agentID.String()+"/sandbox/upgrades/"+upgradeID.String(), nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   role,
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestAgentSandboxUpgrade_StartRequiresAdmin(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedAgentAgent(t, m)
	h.seedSandbox(t, m, agent.ID)

	rr := h.postUpgrade(t, m, agent.ID, nil, "member")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentSandboxUpgrade_StartEnqueuesOperation(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)
	agent := h.seedAgentAgent(t, m)
	oldSandbox := h.seedSandbox(t, m, agent.ID)

	rr := h.postUpgrade(t, m, agent.ID, nil, "admin")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	upgradeID, err := uuid.Parse(resp["upgrade_id"].(string))
	if err != nil {
		t.Fatalf("parse upgrade id: %v", err)
	}
	var upgrade model.AgentSandboxUpgrade
	if err := h.db.First(&upgrade, "id = ?", upgradeID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if upgrade.OldSandboxID == nil || *upgrade.OldSandboxID != oldSandbox.ID {
		t.Fatalf("old sandbox id = %v, want %s", upgrade.OldSandboxID, oldSandbox.ID)
	}
	if upgrade.Status != model.AgentSandboxUpgradeStatusQueued || upgrade.Phase != model.AgentSandboxUpgradePhaseQueued {
		t.Fatalf("status/phase = %s/%s", upgrade.Status, upgrade.Phase)
	}
	h.enqueuer.AssertEnqueued(t, tasks.TypeAgentSandboxUpgrade)
	deleted := h.enqueuer.DeletedTasks()
	if len(deleted) != 1 {
		t.Fatalf("deleted stale tasks = %d, want 1", len(deleted))
	}
	if deleted[0].Queue != tasks.QueueBulk || deleted[0].ID != tasks.AgentSandboxUpgradeTaskID(agent.ID) {
		t.Fatalf("deleted stale task = %#v", deleted[0])
	}
}

func TestAgentSandboxUpgrade_DuplicateActiveReturnsExisting(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)
	agent := h.seedAgentAgent(t, m)
	oldSandbox := h.seedSandbox(t, m, agent.ID)
	existing := model.AgentSandboxUpgrade{
		OrgID:        m.org.ID,
		AgentID:      agent.ID,
		OldSandboxID: &oldSandbox.ID,
		Status:       model.AgentSandboxUpgradeStatusRunning,
		Phase:        model.AgentSandboxUpgradePhaseCreatingNew,
	}
	if err := h.db.Create(&existing).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}

	rr := h.postUpgrade(t, m, agent.ID, nil, "admin")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp["upgrade_id"] != existing.ID.String() {
		t.Fatalf("upgrade_id = %s, want %s", resp["upgrade_id"], existing.ID)
	}
	if got := h.enqueuer.Tasks(); len(got) != 0 {
		t.Fatalf("expected no task enqueued for duplicate, got %d", len(got))
	}
	if got := h.enqueuer.DeletedTasks(); len(got) != 0 {
		t.Fatalf("expected no task deletion for active upgrade, got %d", len(got))
	}
}

func TestAgentSandboxUpgrade_DeletesStaleTaskAfterFailedUpgrade(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)
	agent := h.seedAgentAgent(t, m)
	oldSandbox := h.seedSandbox(t, m, agent.ID)
	failed := model.AgentSandboxUpgrade{
		OrgID:        m.org.ID,
		AgentID:      agent.ID,
		OldSandboxID: &oldSandbox.ID,
		Status:       model.AgentSandboxUpgradeStatusFailed,
		Phase:        model.AgentSandboxUpgradePhaseCreatingNew,
	}
	if err := h.db.Create(&failed).Error; err != nil {
		t.Fatalf("create failed upgrade: %v", err)
	}

	rr := h.postUpgrade(t, m, agent.ID, nil, "admin")
	if rr.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", rr.Code, rr.Body.String())
	}
	deleted := h.enqueuer.DeletedTasks()
	if len(deleted) != 1 {
		t.Fatalf("deleted stale tasks = %d, want 1", len(deleted))
	}
	if deleted[0].Queue != tasks.QueueBulk || deleted[0].ID != tasks.AgentSandboxUpgradeTaskID(agent.ID) {
		t.Fatalf("deleted stale task = %#v", deleted[0])
	}
	h.enqueuer.AssertEnqueued(t, tasks.TypeAgentSandboxUpgrade)
}

func TestAgentSandboxUpgrade_MissingProfileOrSandbox(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)
	agent := h.seedAgentAgent(t, m)

	if rr := h.postUpgrade(t, m, agent.ID, nil, "admin"); rr.Code != http.StatusConflict {
		t.Fatalf("missing sandbox: expected 409, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestAgentSandboxUpgrade_StatusScopedByOrgAndAgent(t *testing.T) {
	h := newAgentHarness(t)
	owner := h.createOrg(t)
	other := h.createOrg(t)
	agent := h.seedAgentAgent(t, owner)
	oldSandbox := h.seedSandbox(t, owner, agent.ID)
	upgrade := model.AgentSandboxUpgrade{
		OrgID:        owner.org.ID,
		AgentID:      agent.ID,
		OldSandboxID: &oldSandbox.ID,
		Status:       model.AgentSandboxUpgradeStatusQueued,
		Phase:        model.AgentSandboxUpgradePhaseQueued,
	}
	if err := h.db.Create(&upgrade).Error; err != nil {
		t.Fatalf("create upgrade: %v", err)
	}

	if rr := h.getUpgrade(t, owner, agent.ID, upgrade.ID, "admin"); rr.Code != http.StatusOK {
		t.Fatalf("owner status: expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr := h.getUpgrade(t, other, agent.ID, upgrade.ID, "admin"); rr.Code != http.StatusNotFound {
		t.Fatalf("other org status: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
	if rr := h.getUpgrade(t, owner, uuid.New(), upgrade.ID, "admin"); rr.Code != http.StatusNotFound {
		t.Fatalf("wrong agent status: expected 404, got %d: %s", rr.Code, rr.Body.String())
	}
}
