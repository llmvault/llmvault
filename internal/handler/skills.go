package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type SkillHandler struct{ db *gorm.DB }

func NewSkillHandler(db *gorm.DB) *SkillHandler { return &SkillHandler{db: db} }

type skillMutationRequest struct {
	TeamID           *string    `json:"team_id,omitempty"`
	Slug             string     `json:"slug"`
	Name             string     `json:"name"`
	Description      *string    `json:"description,omitempty"`
	HumanDescription *string    `json:"human_description,omitempty"`
	Bundle           model.JSON `json:"bundle"`
	Status           string     `json:"status,omitempty"`
}

type skillResponse struct {
	ID               string          `json:"id"`
	OrgID            *string         `json:"org_id,omitempty"`
	TeamID           *string         `json:"team_id,omitempty"`
	Slug             string          `json:"slug"`
	Name             string          `json:"name"`
	Description      *string         `json:"description,omitempty"`
	HumanDescription *string         `json:"human_description,omitempty"`
	SourceType       string          `json:"source_type"`
	Bundle           json.RawMessage `json:"bundle"`
	Status           string          `json:"status"`
}

type skillsResponse struct {
	Skills []skillResponse `json:"skills"`
}
type skillEnvelope struct {
	Skill skillResponse `json:"skill"`
}

func (h *SkillHandler) Mount(r chi.Router) {
	r.Get("/skills", h.List)
	r.Post("/skills", h.Create)
	r.Patch("/skills/{id}", h.Update)
	r.Delete("/skills/{id}", h.Delete)
}

func skillToResponse(skill model.Skill) skillResponse {
	out := skillResponse{ID: skill.ID.String(), Slug: skill.Slug, Name: skill.Name, Description: skill.Description, HumanDescription: skill.HumanDescription, SourceType: skill.SourceType, Bundle: json.RawMessage(skill.Bundle), Status: skill.Status}
	if skill.OrgID != nil {
		value := skill.OrgID.String()
		out.OrgID = &value
	}
	if skill.TeamID != nil {
		value := skill.TeamID.String()
		out.TeamID = &value
	}
	return out
}

func marshalSkillBundle(value model.JSON) model.RawJSON {
	raw, _ := json.Marshal(value)
	return model.RawJSON(raw)
}

// List handles GET /v1/skills.
// @Summary List skills
// @Tags skills
// @Produce json
// @Success 200 {object} skillsResponse
// @Router /v1/skills [get]
func (h *SkillHandler) List(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	q := h.db.WithContext(r.Context()).Where("status <> ? AND org_id = ?", model.SkillStatusArchived, org.ID)
	if !actor.IsOrgManager() {
		q = q.Where("team_id IS NULL OR team_id IN (SELECT team_id FROM team_members WHERE user_id = ? AND deactivated_at IS NULL)", actor.UserID)
	}
	var rows []model.Skill
	if err := q.Order("name ASC").Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list skills"})
		return
	}
	out := make([]skillResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, skillToResponse(row))
	}
	writeJSON(w, http.StatusOK, skillsResponse{Skills: out})
}

// Create handles POST /v1/skills.
// @Summary Create a skill
// @Tags skills
// @Accept json
// @Produce json
// @Param body body skillMutationRequest true "Skill"
// @Success 201 {object} skillEnvelope
// @Router /v1/skills [post]
func (h *SkillHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, actor, ok := h.actor(w, r)
	if !ok {
		return
	}
	var req skillMutationRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	name, slug := strings.TrimSpace(req.Name), strings.TrimSpace(req.Slug)
	if name == "" || slug == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name and slug are required"})
		return
	}
	var teamID *uuid.UUID
	if req.TeamID != nil {
		id, err := uuid.Parse(strings.TrimSpace(*req.TeamID))
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid team_id"})
			return
		}
		allowed, err := actor.CanManageTeamResource(r.Context(), h.db, id)
		if err != nil || !allowed {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
			return
		}
		teamID = &id
	} else if !actor.IsOrgManager() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "only org admins can create org-wide skills"})
		return
	}
	status := strings.TrimSpace(req.Status)
	if status == "" {
		status = model.SkillStatusPublished
	}
	publisher := actor.UserID
	skill := model.Skill{OrgID: &org.ID, TeamID: teamID, PublisherID: &publisher, Slug: slug, Name: name, Description: req.Description, HumanDescription: req.HumanDescription, SourceType: model.SkillSourceInline, Bundle: marshalSkillBundle(req.Bundle), Status: status}
	if len(req.Bundle) == 0 {
		skill.Bundle = model.RawJSON(`{}`)
	}
	if err := h.db.WithContext(r.Context()).Create(&skill).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "skill slug already exists in this scope"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create skill"})
		return
	}
	writeJSON(w, http.StatusCreated, skillEnvelope{Skill: skillToResponse(skill)})
}

// Update handles PATCH /v1/skills/{id}.
// @Summary Update a skill
// @Tags skills
// @Accept json
// @Produce json
// @Param id path string true "Skill ID"
// @Param body body skillMutationRequest true "Skill patch"
// @Success 200 {object} skillEnvelope
// @Router /v1/skills/{id} [patch]
func (h *SkillHandler) Update(w http.ResponseWriter, r *http.Request) {
	_, actor, skill, ok := h.mutableSkill(w, r)
	if !ok {
		return
	}
	var req skillMutationRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	_ = actor
	updates := map[string]any{}
	if value := strings.TrimSpace(req.Name); value != "" {
		updates["name"] = value
	}
	if value := strings.TrimSpace(req.Slug); value != "" {
		updates["slug"] = value
	}
	if req.Description != nil {
		updates["description"] = req.Description
	}
	if req.HumanDescription != nil {
		updates["human_description"] = req.HumanDescription
	}
	if len(req.Bundle) > 0 {
		updates["bundle"] = marshalSkillBundle(req.Bundle)
	}
	if req.Status != "" {
		updates["status"] = req.Status
	}
	if err := h.db.WithContext(r.Context()).Model(&skill).Updates(updates).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update skill"})
		return
	}
	h.db.WithContext(r.Context()).First(&skill, "id = ?", skill.ID)
	writeJSON(w, http.StatusOK, skillEnvelope{Skill: skillToResponse(skill)})
}

// Delete handles DELETE /v1/skills/{id}.
// @Summary Archive a skill
// @Tags skills
// @Param id path string true "Skill ID"
// @Success 200 {object} statusResponse
// @Router /v1/skills/{id} [delete]
func (h *SkillHandler) Delete(w http.ResponseWriter, r *http.Request) {
	_, _, skill, ok := h.mutableSkill(w, r)
	if !ok {
		return
	}
	if err := h.db.WithContext(r.Context()).Model(&skill).Update("status", model.SkillStatusArchived).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to archive skill"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "archived"})
}

func (h *SkillHandler) actor(w http.ResponseWriter, r *http.Request) (*model.Org, *access.Actor, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, nil, false
	}
	userID := strings.TrimSpace(middleware.UserID(r.Context()))
	if userID == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return nil, nil, false
	}
	actor, err := access.Resolve(r.Context(), h.db, org.ID, userID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid actor"})
		return nil, nil, false
	}
	return org, actor, true
}
func (h *SkillHandler) mutableSkill(w http.ResponseWriter, r *http.Request) (*model.Org, *access.Actor, model.Skill, bool) {
	org, actor, ok := h.actor(w, r)
	if !ok {
		return nil, nil, model.Skill{}, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid skill id"})
		return nil, nil, model.Skill{}, false
	}
	var skill model.Skill
	err = h.db.WithContext(r.Context()).Where("id = ? AND org_id = ?", id, org.ID).First(&skill).Error
	if errors.Is(err, gorm.ErrRecordNotFound) || skill.TeamID == nil && !actor.IsOrgManager() {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "skill not found"})
		return nil, nil, model.Skill{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load skill"})
		return nil, nil, model.Skill{}, false
	}
	if skill.TeamID != nil {
		allowed, gateErr := actor.CanManageTeamResource(r.Context(), h.db, *skill.TeamID)
		if gateErr != nil || !allowed {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "skill not found"})
			return nil, nil, model.Skill{}, false
		}
	}
	return org, actor, skill, true
}
