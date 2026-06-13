package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *employeeHarness) getEmployee(t *testing.T, m orgWithMember, agentID string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", "/v1/employees/"+agentID, nil)
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

func TestIntegration_EmployeesGet_HappyPath_LoadsAgentAndSandbox(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, emp.ID)

	rr := h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}

	var item map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if item["id"] != emp.ID.String() {
		t.Fatalf("id = %v, want %s", item["id"], emp.ID)
	}
	if _, exposed := item["profiles"]; exposed {
		t.Fatalf("employee response exposed removed profiles field: %#v", item["profiles"])
	}
	if _, ok := item["sandbox"].(map[string]any); !ok {
		t.Fatalf("sandbox missing: %#v", item["sandbox"])
	}
}

func TestIntegration_EmployeesGet_EnsuresMissingMemoryBank(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)

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

	rr := h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	if configCalls != 1 || mentalModelCalls != 1 {
		t.Fatalf("hindsight calls config=%d mental_model=%d, want 1/1", configCalls, mentalModelCalls)
	}
	var bank model.HindsightBank
	if err := h.db.First(&bank, "bank_id = ?", hindsight.OrgBankID(m.org.ID)).Error; err != nil {
		t.Fatalf("load memory bank tracker: %v", err)
	}
}

func TestIntegration_EmployeesGet_ReportsSandboxUpgradeAvailability(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	m := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, m)
	sb := h.seedSandbox(t, m, emp.ID)
	h.setSandboxSnapshot(t, sb.ID, &h.cfg.SandboxesRuntimeBaseImage)

	rr := h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	var item map[string]any
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode matching snapshot response: %v", err)
	}
	if item["upgrade_available"] != false {
		t.Fatalf("matching sandbox upgrade_available = %v, want false", item["upgrade_available"])
	}
	if _, exposed := item["snapshot_id"]; exposed {
		t.Fatalf("employee response exposed snapshot_id: %#v", item)
	}

	outdatedSnapshot := "older-employee-sandbox"
	h.setSandboxSnapshot(t, sb.ID, &outdatedSnapshot)
	rr = h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("outdated status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	item = map[string]any{}
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode outdated snapshot response: %v", err)
	}
	if item["upgrade_available"] != true {
		t.Fatalf("outdated sandbox upgrade_available = %v, want true", item["upgrade_available"])
	}

	h.setSandboxSnapshot(t, sb.ID, nil)
	rr = h.getEmployee(t, m, emp.ID.String())
	if rr.Code != http.StatusOK {
		t.Fatalf("missing snapshot status = %d, want 200: %s", rr.Code, rr.Body.String())
	}
	item = map[string]any{}
	if err := json.Unmarshal(rr.Body.Bytes(), &item); err != nil {
		t.Fatalf("decode missing snapshot response: %v", err)
	}
	if item["upgrade_available"] != true {
		t.Fatalf("missing snapshot upgrade_available = %v, want true", item["upgrade_available"])
	}
}

func TestIntegration_EmployeesGet_ScopedToOrg(t *testing.T) {
	h := newEmployeeHarness(t)
	h.platformCredCleanup(t)
	owner := h.createOrg(t)
	stranger := h.createOrg(t)
	emp := h.seedEmployeeAgent(t, owner)

	rr := h.getEmployee(t, stranger, emp.ID.String())
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404: %s", rr.Code, rr.Body.String())
	}
}
