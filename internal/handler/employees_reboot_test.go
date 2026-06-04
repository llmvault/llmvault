package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeHandler_RebootSandboxRestartsAndSyncsRuntimeConfig(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	sb := h.seedSandbox(t, m, agent.ID)
	drive := model.Skill{
		Slug:       "drive",
		Name:       "drive",
		SourceType: model.SkillSourceInline,
		Status:     model.SkillStatusPublished,
		Bundle:     model.RawJSON(`{"description":"Drive files.","content":"Use HIVY_DRIVE_UPLOAD_URL.","files":{}}`),
	}
	if err := h.db.Create(&drive).Error; err != nil {
		t.Fatalf("seed drive skill: %v", err)
	}

	rr := h.rebootEmployeeSandbox(t, m, agent.ID, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	if h.provider.restartCount != 1 {
		t.Fatalf("restart count = %d, want 1", h.provider.restartCount)
	}
	calls, bearer := h.sidecar.snapshot()
	if calls != 1 {
		t.Fatalf("config push calls = %d, want 1", calls)
	}
	if bearer == "" {
		t.Fatal("config push missing runtime auth bearer")
	}
	envBody := h.sidecar.envBody()
	for _, key := range []string{
		"HIVY_RAILWAY_API_URL",
		"HIVY_RAILWAY_API_KEY",
		"HIVY_VERCEL_API_URL",
		"HIVY_VERCEL_API_KEY",
		"HIVY_DRIVE_UPLOAD_URL",
		"HIVY_DRIVE_UPLOAD_BEARER",
	} {
		if !strings.Contains(string(envBody), key) {
			t.Fatalf("runtime env body missing %s: %s", key, string(envBody))
		}
	}
	configBody := h.sidecar.configBody()
	if !strings.Contains(string(configBody), `"name":"drive"`) {
		t.Fatalf("runtime config missing reconciled drive skill: %s", string(configBody))
	}
	var links int64
	if err := h.db.Model(&model.EmployeeSkill{}).
		Where("employee_id = ? AND skill_id = ?", agent.ID, drive.ID).
		Count(&links).Error; err != nil {
		t.Fatalf("count drive link: %v", err)
	}
	if links != 1 {
		t.Fatalf("drive employee skill links = %d, want 1", links)
	}

	var resp struct {
		SandboxID string `json:"sandbox_id"`
		Employee  struct {
			ID string `json:"id"`
		} `json:"employee"`
		Sync struct {
			Applied int `json:"applied"`
		} `json:"sync"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.SandboxID != sb.ID.String() || resp.Employee.ID != agent.ID.String() || resp.Sync.Applied != 1 {
		t.Fatalf("response = %#v", resp)
	}

	var refreshed model.Sandbox
	if err := h.db.Where("id = ?", sb.ID).First(&refreshed).Error; err != nil {
		t.Fatalf("reload sandbox: %v", err)
	}
	if refreshed.Status != "running" || refreshed.StoppedAt != nil || refreshed.ErrorMessage != nil {
		t.Fatalf("sandbox status after reboot = status:%q stopped:%v error:%v", refreshed.Status, refreshed.StoppedAt, refreshed.ErrorMessage)
	}
}

func TestEmployeeHandler_RebootSandboxEnsuresMissingMemoryBank(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)

	var configCalls int
	var mentalModelCalls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && strings.HasSuffix(r.URL.Path, "/config"):
			configCalls++
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/mental-models"):
			mentalModelCalls++
			w.WriteHeader(http.StatusCreated)
		default:
			t.Fatalf("unexpected hindsight request: %s %s", r.Method, r.URL.Path)
		}
	}))
	t.Cleanup(srv.Close)
	h.handler.SetMemoryProvisioner(hindsight.NewBankProvisioner(h.db, hindsight.NewClient(srv.URL)))

	rr := h.rebootEmployeeSandbox(t, m, agent.ID, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}
	if configCalls != 1 || mentalModelCalls != 1 {
		t.Fatalf("hindsight calls config=%d mental_model=%d, want 1/1", configCalls, mentalModelCalls)
	}
	var bank model.HindsightBank
	if err := h.db.First(&bank, "bank_id = ?", hindsight.OrgBankID(m.org.ID)).Error; err != nil {
		t.Fatalf("load memory bank tracker: %v", err)
	}
}

func TestEmployeeHandler_RebootSandboxRequiresAdmin(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)

	rr := h.rebootEmployeeSandbox(t, m, agent.ID, "member")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rr.Code, rr.Body.String())
	}
}
