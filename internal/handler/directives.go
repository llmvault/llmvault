package handler

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type DirectiveHandler struct{ db *gorm.DB }

func NewDirectiveHandler(db *gorm.DB) *DirectiveHandler { return &DirectiveHandler{db: db} }

type directiveMutationRequest struct {
	AgentID string `json:"agent_id"`
	Content string `json:"content"`
	Active  *bool  `json:"active,omitempty"`
}
type directiveResponse struct {
	ID        string `json:"id"`
	AgentID   string `json:"agent_id"`
	Content   string `json:"content"`
	Source    string `json:"source"`
	Active    bool   `json:"active"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}
type directiveListResponse struct {
	Data []directiveResponse `json:"data"`
}

// List handles GET /v1/directives?agent_id=UUID.
// @Router /v1/directives [get]
func (h *DirectiveHandler) List(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := directiveContext(w, r)
	if !ok {
		return
	}
	agentID, ok := directiveAgentID(w, r)
	if !ok {
		return
	}
	if !directiveCanManage(r, h.db, org.ID, userID, agentID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}
	var rows []model.AgentDirective
	if err := h.db.WithContext(r.Context()).Where("org_id = ? AND agent_id = ? AND deleted_at IS NULL", org.ID, agentID).Order("created_at ASC").Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list directives"})
		return
	}
	out := make([]directiveResponse, len(rows))
	for i := range rows {
		out[i] = directiveToResponse(rows[i])
	}
	writeJSON(w, http.StatusOK, directiveListResponse{Data: out})
}

// Create handles POST /v1/directives.
// @Router /v1/directives [post]
func (h *DirectiveHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := directiveContext(w, r)
	if !ok {
		return
	}
	var req directiveMutationRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	agentID, ok := directiveAgentIDValue(w, req.AgentID)
	if !ok {
		return
	}
	if !directiveCanManage(r, h.db, org.ID, userID, agentID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "content is required"})
		return
	}
	row := model.AgentDirective{OrgID: org.ID, AgentID: agentID, Content: content, Source: model.DirectiveSourceUserPinned, Active: true, CreatedByUserID: userID}
	if err := h.db.WithContext(r.Context()).Create(&row).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create directive"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]directiveResponse{"directive": directiveToResponse(row)})
}

// Update handles PATCH /v1/directives/{id}.
// @Router /v1/directives/{id} [patch]
func (h *DirectiveHandler) Update(w http.ResponseWriter, r *http.Request) {
	row, ok := h.authorized(w, r)
	if !ok {
		return
	}
	var req directiveMutationRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if req.Active != nil {
		if err := h.db.WithContext(r.Context()).Model(&row).Update("active", *req.Active).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update directive"})
			return
		}
		row.Active = *req.Active
		row.UpdatedAt = time.Now()
	}
	writeJSON(w, http.StatusOK, map[string]directiveResponse{"directive": directiveToResponse(row)})
}

// Delete handles DELETE /v1/directives/{id}.
// @Router /v1/directives/{id} [delete]
func (h *DirectiveHandler) Delete(w http.ResponseWriter, r *http.Request) {
	row, ok := h.authorized(w, r)
	if !ok {
		return
	}
	now := time.Now()
	if err := h.db.WithContext(r.Context()).Model(&row).Update("deleted_at", now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete directive"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "deleted"})
}
func (h *DirectiveHandler) authorized(w http.ResponseWriter, r *http.Request) (model.AgentDirective, bool) {
	org, userID, ok := directiveContext(w, r)
	if !ok {
		return model.AgentDirective{}, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid directive id"})
		return model.AgentDirective{}, false
	}
	var row model.AgentDirective
	if h.db.WithContext(r.Context()).Where("id = ? AND org_id = ? AND deleted_at IS NULL", id, org.ID).First(&row).Error != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "directive not found"})
		return row, false
	}
	if !directiveCanManage(r, h.db, org.ID, userID, row.AgentID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return row, false
	}
	return row, true
}
func directiveContext(w http.ResponseWriter, r *http.Request) (*model.Org, *uuid.UUID, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, nil, false
	}
	userID, _ := currentRequestUserID(r.Context())
	return org, userID, true
}
func directiveAgentID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	return directiveAgentIDValue(w, r.URL.Query().Get("agent_id"))
}
func directiveAgentIDValue(w http.ResponseWriter, raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(strings.TrimSpace(raw))
	if err != nil || id == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent_id must be a uuid"})
		return uuid.Nil, false
	}
	return id, true
}
func directiveCanManage(r *http.Request, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID, agentID uuid.UUID) bool {
	var agent model.Agent
	if db.WithContext(r.Context()).Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").First(&agent).Error != nil {
		return false
	}
	return canUseTeam(r.Context(), db, orgID, agent.TeamID, userID, isAPIKeyRequest(r.Context()))
}
func directiveToResponse(row model.AgentDirective) directiveResponse {
	return directiveResponse{ID: row.ID.String(), AgentID: row.AgentID.String(), Content: row.Content, Source: row.Source, Active: row.Active, CreatedAt: row.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: row.UpdatedAt.UTC().Format(time.RFC3339)}
}
