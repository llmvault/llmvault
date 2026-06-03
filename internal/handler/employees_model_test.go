package handler_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/employeeruntime"
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

	h := handler.NewEmployeeHandler(db, nil, employeeruntime.CompileDeps{}, registry.Global())
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
