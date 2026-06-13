package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
)

func TestEmployeeHandler_ListModelsReturnsEmployeeOpenRouterAllowlist(t *testing.T) {
	db := connectTestDB(t)
	cred := model.Credential{
		Label:        "test-openrouter-employee-models",
		BaseURL:      "https://openrouter.ai/api/v1",
		AuthScheme:   "bearer",
		ProviderID:   "openrouter",
		EncryptedKey: []byte("enc"),
		WrappedDEK:   []byte("dek"),
	}
	if err := db.Create(&cred).Error; err != nil {
		t.Fatalf("create credential: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", cred.ID).Delete(&model.Credential{})
	})

	h := handler.NewEmployeeHandler(db, nil, agentruntime.CompileDeps{}, registry.Global())
	req := httptest.NewRequest(http.MethodGet, "/v1/employees/models", nil)
	rr := httptest.NewRecorder()
	h.ListModels(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var models []struct {
		ID          string   `json:"id"`
		Name        string   `json:"name"`
		ProviderIDs []string `json:"provider_ids"`
		Cost        struct {
			Input     float64 `json:"input"`
			Output    float64 `json:"output"`
			CacheRead float64 `json:"cache_read"`
		} `json:"cost"`
		Limit struct {
			Context int64 `json:"context"`
			Output  int64 `json:"output"`
		} `json:"limit"`
		Description string `json:"description"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &models); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	wantIDs := []string{"deepseek-v4-flash", "step-3.7-flash", "ling-2.6-1t", "mimo-v2.5-pro"}
	if len(models) != len(wantIDs) {
		t.Fatalf("model count = %d, want %d: %#v", len(models), len(wantIDs), models)
	}
	for i, want := range wantIDs {
		if models[i].ID != want {
			t.Fatalf("models[%d].id = %q, want %q", i, models[i].ID, want)
		}
		if len(models[i].ProviderIDs) != 1 || models[i].ProviderIDs[0] != "openrouter" {
			t.Fatalf("models[%d].provider_ids = %#v, want [openrouter]", i, models[i].ProviderIDs)
		}
		if models[i].Description == "" {
			t.Fatalf("models[%d].description is empty", i)
		}
	}
	if models[1].Name != "Step 3.7 Flash" || models[1].Cost.Input != 0.2 || models[1].Cost.Output != 1.15 || models[1].Cost.CacheRead != 0.04 {
		t.Fatalf("step model mismatch: %#v", models[1])
	}
	if models[2].Name != "Ling-2.6-1T" || models[2].Cost.Input != 0.075 || models[2].Cost.Output != 0.625 || models[2].Limit.Output != 32768 {
		t.Fatalf("ling model mismatch: %#v", models[2])
	}
	if models[0].Cost.Input != 0.0983 || models[0].Cost.Output != 0.1966 || models[0].Cost.CacheRead != 0.0197 {
		t.Fatalf("deepseek pricing mismatch: %#v", models[0])
	}
	if models[3].Cost.Input != 0.435 || models[3].Cost.Output != 0.87 || models[3].Cost.CacheRead != 0.0036 {
		t.Fatalf("mimo pricing mismatch: %#v", models[3])
	}
}

func TestEmployeeHandler_UpdateModelPersistsAndPushesRuntimeConfig(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)
	h.seedSandbox(t, m, agent.ID)

	rr := h.patchEmployeeModel(t, m, agent.ID, map[string]any{
		"model": "step-3.7-flash",
	}, "admin")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body=%s", rr.Code, rr.Body.String())
	}

	var updated model.Employee
	if err := h.db.Where("id = ?", agent.ID).First(&updated).Error; err != nil {
		t.Fatalf("reload employee: %v", err)
	}
	if updated.Model != "step-3.7-flash" {
		t.Fatalf("employee model = %q, want step-3.7-flash", updated.Model)
	}

	var pushed struct {
		Model struct {
			ModelID string `json:"model_id"`
		} `json:"model"`
	}
	if err := json.Unmarshal(h.sidecar.configBody(), &pushed); err != nil {
		t.Fatalf("decode pushed runtime config: %v", err)
	}
	if pushed.Model.ModelID != "step-3.7-flash" {
		t.Fatalf("pushed model_id = %q, want step-3.7-flash", pushed.Model.ModelID)
	}

	var resp struct {
		Employee struct {
			Model string `json:"model"`
		} `json:"employee"`
		Sync struct {
			Applied int `json:"applied"`
		} `json:"sync"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if resp.Employee.Model != "step-3.7-flash" || resp.Sync.Applied != 1 {
		t.Fatalf("response = %#v", resp)
	}
}

func TestEmployeeHandler_UpdateModelRejectsNonEmployeeModel(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrg(t)
	agent := h.seedEmployeeAgent(t, m)

	rr := h.patchEmployeeModel(t, m, agent.ID, map[string]any{
		"model": "deepseek-v4-pro",
	}, "admin")
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body=%s", rr.Code, rr.Body.String())
	}
}

func TestEmployeeHandler_UpdateModelRequiresAdmin(t *testing.T) {
	h := newEmployeeHarness(t)
	m := h.createOrgWithRole(t, "member")
	agent := h.seedEmployeeAgent(t, m)

	rr := h.patchEmployeeModel(t, m, agent.ID, map[string]any{
		"model": "step-3.7-flash",
	}, "member")
	if rr.Code != http.StatusForbidden {
		t.Fatalf("status = %d, want 403, body=%s", rr.Code, rr.Body.String())
	}
}
