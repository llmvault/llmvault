package handler

import (
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *CanvasHandler) ListAgentBrands(w http.ResponseWriter, r *http.Request) {
	brandHandler, req, ok := h.runtimeBrandRequest(w, r)
	if !ok {
		return
	}
	brandHandler.List(w, req)
}

func (h *CanvasHandler) GetAgentBrand(w http.ResponseWriter, r *http.Request) {
	brandHandler, req, ok := h.runtimeBrandRequest(w, r)
	if !ok {
		return
	}
	brandHandler.Get(w, req)
}

func (h *CanvasHandler) CreateAgentBrand(w http.ResponseWriter, r *http.Request) {
	brandHandler, req, ok := h.runtimeBrandRequest(w, r)
	if !ok {
		return
	}
	brandHandler.Create(w, req)
}

func (h *CanvasHandler) UpdateAgentBrand(w http.ResponseWriter, r *http.Request) {
	brandHandler, req, ok := h.runtimeBrandRequest(w, r)
	if !ok {
		return
	}
	brandHandler.Update(w, req)
}

func (h *CanvasHandler) runtimeBrandRequest(w http.ResponseWriter, r *http.Request) (*BrandHandler, *http.Request, bool) {
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
		return nil, nil, false
	}
	orgID, ok := h.runtimeAgentOrgID(w, r, agentID)
	if !ok {
		return nil, nil, false
	}
	req := middleware.WithOrg(r, &model.Org{ID: orgID})
	return NewBrandHandler(h.db), req, true
}

func (h *CanvasHandler) runtimeAgentOrgID(w http.ResponseWriter, r *http.Request, agentID uuid.UUID) (uuid.UUID, bool) {
	var agent model.Agent
	err := h.db.WithContext(r.Context()).Select("org_id").Where("id = ?", agentID).First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
		return uuid.Nil, false
	}
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "canvas runtime brand agent lookup", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load agent"})
		return uuid.Nil, false
	}
	if agent.OrgID == nil || *agent.OrgID == uuid.Nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent org not found"})
		return uuid.Nil, false
	}
	return *agent.OrgID, true
}
