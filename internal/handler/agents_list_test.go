package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *agentHarness) listAgents(t *testing.T, m orgWithMember) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/agents", nil)
	req.Header.Set("X-Org-ID", m.org.ID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: m.user.ID.String(),
		OrgID:  m.org.ID.String(),
		Role:   "admin",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func TestIntegration_AgentsList_HappyPath_LoadsAllRelations(t *testing.T) {
	h := newAgentHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedAgentAgent(t, m)
	h.seedSandbox(t, m, emp.ID)

	skill := model.Skill{
		Slug: "list-skill-" + randSuffix(),
		Name: "List Skill", SourceType: model.SkillSourceInline,
		Status: model.SkillStatusPublished,
	}
	if err := h.db.Create(&skill).Error; err != nil {
		t.Fatalf("create skill: %v", err)
	}
	t.Cleanup(func() { h.db.Where("id = ?", skill.ID).Delete(&model.Skill{}) })
	if err := h.db.Create(&model.AgentSkill{AgentID: emp.ID, SkillID: skill.ID}).Error; err != nil {
		t.Fatalf("attach skill: %v", err)
	}
	archivedSkill := model.Skill{
		Slug: "archived-list-skill-" + randSuffix(),
		Name: "Archived List Skill", SourceType: model.SkillSourceInline,
		Status: model.SkillStatusArchived,
	}
	if err := h.db.Create(&archivedSkill).Error; err != nil {
		t.Fatalf("create archived skill: %v", err)
	}
	if err := h.db.Create(&model.AgentSkill{AgentID: emp.ID, SkillID: archivedSkill.ID}).Error; err != nil {
		t.Fatalf("attach archived skill: %v", err)
	}
	t.Cleanup(func() { h.db.Where("agent_id = ?", emp.ID).Delete(&model.AgentSkill{}) })

	rr := h.listAgents(t, m)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Data) != 1 {
		t.Fatalf("data len = %d, want 1", len(resp.Data))
	}
	item := resp.Data[0]

	if item["id"] != emp.ID.String() {
		t.Errorf("id mismatch: got %v", item["id"])
	}

	attached := item["attached_skills"].([]any)
	if len(attached) != 1 {
		t.Errorf("attached_skills len = %d, want 1", len(attached))
	} else {
		sk := attached[0].(map[string]any)
		if sk["name"] != "List Skill" {
			t.Errorf("skill name = %v, want List Skill", sk["name"])
		}
		// Skill summary must NOT carry bundle content.
		if _, hasContent := sk["content"]; hasContent {
			t.Errorf("skill summary leaked content field")
		}
	}

	if _, exposed := item["profiles"]; exposed {
		t.Fatalf("agent list response exposed removed profiles field: %#v", item["profiles"])
	}
	for _, key := range []string{"is_managed", "category", "system_prompt", "identity_prompt", "prompt_operating_principles", "integrations", "agent_config"} {
		if _, exposed := item[key]; exposed {
			t.Fatalf("agent list response exposed removed %s field", key)
		}
	}

	sb := item["sandbox"].(map[string]any)
	if sb["status"] != "running" {
		t.Errorf("sandbox.status = %v, want running", sb["status"])
	}
}

func TestIntegration_AgentsList_ReportsSandboxUpgradeAvailability(t *testing.T) {
	h := newAgentHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)

	matching := h.seedAgentAgent(t, m)
	matchingSandbox := h.seedSandbox(t, m, matching.ID)
	h.setSandboxSnapshot(t, matchingSandbox.ID, &h.cfg.SandboxesRuntimeBaseImage)

	outdated := h.seedAgentAgent(t, m)
	outdatedSandbox := h.seedSandbox(t, m, outdated.ID)
	outdatedSnapshot := "ghcr.io/usehivy/hivy-sandboxes-runtime:v0.0.1"
	h.setSandboxSnapshot(t, outdatedSandbox.ID, &outdatedSnapshot)
	missingSnapshot := h.seedAgentAgent(t, m)
	missingSnapshotSandbox := h.seedSandbox(t, m, missingSnapshot.ID)
	h.setSandboxSnapshot(t, missingSnapshotSandbox.ID, nil)

	rr := h.listAgents(t, m)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var resp struct {
		Data []map[string]any `json:"data"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	got := make(map[string]bool, len(resp.Data))
	selectedSandboxID := make(map[string]string, len(resp.Data))
	for _, item := range resp.Data {
		id, _ := item["id"].(string)
		if _, exposed := item["snapshot_id"]; exposed {
			t.Fatalf("agent response exposed snapshot_id: %#v", item)
		}
		if sandbox, ok := item["sandbox"].(map[string]any); ok {
			if _, exposed := sandbox["snapshot_id"]; exposed {
				t.Fatalf("agent sandbox response exposed snapshot_id: %#v", sandbox)
			}
			selectedSandboxID[id], _ = sandbox["id"].(string)
		}
		upgrade, _ := item["upgrade_available"].(bool)
		got[id] = upgrade
	}

	if got[matching.ID.String()] {
		t.Errorf("matching sandbox upgrade_available = true, want false")
	}
	if !got[outdated.ID.String()] {
		t.Errorf("outdated sandbox upgrade_available = false, want true")
	}
	if selectedSandboxID[outdated.ID.String()] != outdatedSandbox.ID.String() {
		t.Errorf("outdated selected sandbox = %s, want %s", selectedSandboxID[outdated.ID.String()], outdatedSandbox.ID)
	}
	if !got[missingSnapshot.ID.String()] {
		t.Errorf("missing snapshot upgrade_available = false, want true")
	}
}

func TestIntegration_AgentsList_ScopedToOrg(t *testing.T) {
	h := newAgentHarness(t)
	h.platformCredCleanup(t)
	owner := h.createOrg(t)
	stranger := h.createOrg(t)
	h.seedAgentAgent(t, owner)

	rr := h.listAgents(t, stranger)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp struct {
		Data []map[string]any `json:"data"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Fatalf("cross-org: data len = %d, want 0", len(resp.Data))
	}
}

func TestIntegration_AgentsList_NonAdminAllowed(t *testing.T) {
	h := newAgentHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrgWithRole(t, "member")
	h.seedAgentAgent(t, m)

	rr := h.listAgents(t, m)
	if rr.Code != http.StatusOK {
		t.Fatalf("non-admin should read list: status = %d, body=%s", rr.Code, rr.Body.String())
	}
}

func TestIntegration_AgentsList_EmptyOrg(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)

	rr := h.listAgents(t, m)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d", rr.Code)
	}
	var resp struct {
		Data    []any `json:"data"`
		HasMore bool  `json:"has_more"`
	}
	json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Data) != 0 {
		t.Errorf("empty org: data len = %d, want 0", len(resp.Data))
	}
	if resp.HasMore {
		t.Errorf("empty org: has_more = true, want false")
	}
}
