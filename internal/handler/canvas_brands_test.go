package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/model"
)

type runtimeBrandHarness struct {
	db            *gorm.DB
	router        *chi.Mux
	agentID       uuid.UUID
	runtimeSecret string
}

func newRuntimeBrandHarness(t *testing.T) *runtimeBrandHarness {
	t.Helper()
	db := connectTestDB(t)
	encKey := testSymmetricKey(t)
	org := createTestOrg(t, db)
	agent := seedSessionAgent(t, db, org.ID)
	runtimeSecret := "runtime-brand-secret" // #nosec G101 -- test runtime bearer secret.
	encrypted, err := encKey.EncryptString(runtimeSecret)
	if err != nil {
		t.Fatalf("encrypt runtime secret: %v", err)
	}
	sandbox := model.Sandbox{
		ID:                     uuid.New(),
		OrgID:                  &org.ID,
		AgentID:                &agent.ID,
		Status:                 "running",
		EncryptedRuntimeSecret: encrypted,
		RuntimeURL:             "http://localhost:7080",
	}
	if err := db.Create(&sandbox).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.BrandAsset{})
		db.Where("org_id = ?", org.ID).Delete(&model.Brand{})
		db.Where("id = ?", sandbox.ID).Delete(&model.Sandbox{})
		db.Where("id = ?", agent.ID).Delete(&model.Agent{})
	})
	canvasHandler := handler.NewCanvasHandler(db, encKey, nil)
	router := chi.NewRouter()
	router.Get("/internal/agents/{agentID}/canvas/brands", canvasHandler.ListAgentBrands)
	router.Post("/internal/agents/{agentID}/canvas/brands", canvasHandler.CreateAgentBrand)
	router.Get("/internal/agents/{agentID}/canvas/brands/{id}", canvasHandler.GetAgentBrand)
	router.Patch("/internal/agents/{agentID}/canvas/brands/{id}", canvasHandler.UpdateAgentBrand)
	return &runtimeBrandHarness{db: db, router: router, agentID: agent.ID, runtimeSecret: runtimeSecret}
}

func TestIntegration_CanvasBrandsRuntimeCRUD(t *testing.T) {
	h := newRuntimeBrandHarness(t)
	create := h.doJSON(t, http.MethodPost, "/internal/agents/"+h.agentID.String()+"/canvas/brands", h.runtimeSecret, map[string]any{
		"name":        "Canvas Brand",
		"description": "Original",
		"colors": map[string]any{
			"tokens": []any{map[string]any{"id": "primary", "value": "#2463eb"}},
		},
	})
	if create.Code != http.StatusCreated {
		t.Fatalf("create brand status=%d body=%s", create.Code, create.Body.String())
	}
	created := decodeBrandMutation(t, create)

	list := h.doJSON(t, http.MethodGet, "/internal/agents/"+h.agentID.String()+"/canvas/brands", h.runtimeSecret, nil)
	if list.Code != http.StatusOK {
		t.Fatalf("list brands status=%d body=%s", list.Code, list.Body.String())
	}
	var page struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.Unmarshal(list.Body.Bytes(), &page); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if len(page.Data) != 1 || page.Data[0].ID != created.Brand.ID {
		t.Fatalf("unexpected list response: %+v", page.Data)
	}

	view := h.doJSON(t, http.MethodGet, "/internal/agents/"+h.agentID.String()+"/canvas/brands/"+created.Brand.ID, h.runtimeSecret, nil)
	if view.Code != http.StatusOK {
		t.Fatalf("view brand status=%d body=%s", view.Code, view.Body.String())
	}

	patch := h.doJSON(t, http.MethodPatch, "/internal/agents/"+h.agentID.String()+"/canvas/brands/"+created.Brand.ID, h.runtimeSecret, map[string]any{
		"description": "Updated by runtime",
	})
	if patch.Code != http.StatusOK {
		t.Fatalf("patch brand status=%d body=%s", patch.Code, patch.Body.String())
	}
}

func TestIntegration_CanvasBrandsRejectBadRuntimeSecret(t *testing.T) {
	h := newRuntimeBrandHarness(t)
	rr := h.doJSON(t, http.MethodGet, "/internal/agents/"+h.agentID.String()+"/canvas/brands", "wrong-secret", nil)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("wrong runtime secret status=%d body=%s", rr.Code, rr.Body.String())
	}
}

func (h *runtimeBrandHarness) doJSON(t *testing.T, method, path, runtimeSecret string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+runtimeSecret)
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}
