package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// DirectiveHandler serves agent directives: hard rules injected verbatim into
// every agent prompt in scope (channel-scoped or org-wide).
type DirectiveHandler struct {
	db *gorm.DB
}

func NewDirectiveHandler(db *gorm.DB) *DirectiveHandler {
	return &DirectiveHandler{db: db}
}

type directiveMutationRequest struct {
	ChannelID *string `json:"channel_id,omitempty"`
	Content   *string `json:"content,omitempty"`
	Active    *bool   `json:"active,omitempty"`
}

type directiveResponse struct {
	ID              string  `json:"id"`
	ChannelID       *string `json:"channel_id,omitempty"`
	Content         string  `json:"content"`
	Source          string  `json:"source"`
	Active          bool    `json:"active"`
	CreatedByUserID *string `json:"created_by_user_id,omitempty"`
	CreatedAt       string  `json:"created_at"`
	UpdatedAt       string  `json:"updated_at"`
}

type directiveListResponse struct {
	Data []directiveResponse `json:"data"`
}

// canManageMemoryResources mirrors the memories write gate: org admins/owners
// and scoped API keys may mutate memory-control resources.
func canManageMemoryResources(r *http.Request, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) bool {
	if isAPIKeyRequest(r.Context()) {
		return true
	}
	if userID == nil {
		return false
	}
	var membership model.OrgMembership
	if err := db.WithContext(r.Context()).
		Where("org_id = ? AND user_id = ?", orgID, *userID).
		First(&membership).Error; err != nil {
		return false
	}
	return membership.Role == "owner" || membership.Role == "admin"
}

// List handles GET /v1/directives.
// @Summary List directives
// @Description Lists agent directives. channel_id filters: a channel UUID returns that channel's directives plus org-wide ones (the injected set for the channel), "org" returns org-wide only, omitted returns all.
// @Tags directives
// @Produce json
// @Param channel_id query string false "Filter: channel UUID (channel + org-wide), \"org\", or omitted for all"
// @Success 200 {object} directiveListResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/directives [get]
func (h *DirectiveHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := orgForChannelRequest(w, r.Context())
	if !ok {
		return
	}
	query := h.db.WithContext(r.Context()).Where("org_id = ?", org.ID)
	switch raw := strings.TrimSpace(r.URL.Query().Get("channel_id")); raw {
	case "":
	case "org":
		query = query.Where("channel_id IS NULL")
	default:
		channelID, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid channel_id"})
			return
		}
		query = query.Where("channel_id = ? OR channel_id IS NULL", channelID)
	}
	var directives []model.AgentDirective
	if err := query.Order("created_at ASC").Find(&directives).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "list directives", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list directives"})
		return
	}
	out := make([]directiveResponse, len(directives))
	for i := range directives {
		out[i] = directiveToResponse(directives[i])
	}
	writeJSON(w, http.StatusOK, directiveListResponse{Data: out})
}

// Create handles POST /v1/directives.
// @Summary Create a directive
// @Description Creates a manual (user-pinned) directive in a channel (channel_id) or org-wide (omit channel_id). Requires an org admin/owner.
// @Tags directives
// @Accept json
// @Produce json
// @Param request body directiveMutationRequest true "Directive content and optional channel_id"
// @Success 201 {object} map[string]directiveResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/directives [post]
func (h *DirectiveHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := orgForChannelRequest(w, r.Context())
	if !ok {
		return
	}
	userID, _ := currentRequestUserID(r.Context())
	if !canManageMemoryResources(r, h.db, org.ID, userID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "admin role required to manage directives"})
		return
	}
	var req directiveMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	content := ""
	if req.Content != nil {
		content = strings.TrimSpace(*req.Content)
	}
	if content == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "content is required"})
		return
	}
	channelID, ok := h.resolveDirectiveChannel(w, r, org.ID, req.ChannelID)
	if !ok {
		return
	}
	directive := model.AgentDirective{
		OrgID:           org.ID,
		ChannelID:       channelID,
		Content:         content,
		CreatedByUserID: userID,
		Source:          model.DirectiveSourceUserPinned,
		Active:          true,
	}
	if err := h.db.WithContext(r.Context()).Create(&directive).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "create directive", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create directive"})
		return
	}
	writeJSON(w, http.StatusCreated, map[string]directiveResponse{"directive": directiveToResponse(directive)})
}

// Update handles PATCH /v1/directives/{id}.
// @Summary Update a directive
// @Description Updates a directive's content and/or active flag. Requires an org admin/owner.
// @Tags directives
// @Accept json
// @Produce json
// @Param id path string true "Directive UUID"
// @Param request body directiveMutationRequest true "Fields to update"
// @Success 200 {object} map[string]directiveResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/directives/{id} [patch]
func (h *DirectiveHandler) Update(w http.ResponseWriter, r *http.Request) {
	directive, ok := h.authorizeDirectiveMutation(w, r)
	if !ok {
		return
	}
	var req directiveMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	updates := map[string]any{}
	if req.Content != nil {
		content := strings.TrimSpace(*req.Content)
		if content == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "content cannot be empty"})
			return
		}
		updates["content"] = content
		directive.Content = content
	}
	if req.Active != nil {
		updates["active"] = *req.Active
		directive.Active = *req.Active
	}
	if len(updates) > 0 {
		if err := h.db.WithContext(r.Context()).
			Model(&model.AgentDirective{}).
			Where("id = ? AND org_id = ?", directive.ID, directive.OrgID).
			Updates(updates).Error; err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "update directive", "error", err, "directive_id", directive.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update directive"})
			return
		}
		directive.UpdatedAt = time.Now()
	}
	writeJSON(w, http.StatusOK, map[string]directiveResponse{"directive": directiveToResponse(directive)})
}

// Delete handles DELETE /v1/directives/{id}.
// @Summary Delete a directive
// @Description Permanently deletes a directive so it is no longer injected into prompts. Requires an org admin/owner.
// @Tags directives
// @Produce json
// @Param id path string true "Directive UUID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/directives/{id} [delete]
func (h *DirectiveHandler) Delete(w http.ResponseWriter, r *http.Request) {
	directive, ok := h.authorizeDirectiveMutation(w, r)
	if !ok {
		return
	}
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", directive.ID, directive.OrgID).
		Delete(&model.AgentDirective{}).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete directive", "error", err, "directive_id", directive.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete directive"})
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "deleted"})
}

func (h *DirectiveHandler) authorizeDirectiveMutation(w http.ResponseWriter, r *http.Request) (model.AgentDirective, bool) {
	var directive model.AgentDirective
	org, ok := orgForChannelRequest(w, r.Context())
	if !ok {
		return directive, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid directive id"})
		return directive, false
	}
	userID, _ := currentRequestUserID(r.Context())
	if !canManageMemoryResources(r, h.db, org.ID, userID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "admin role required to manage directives"})
		return directive, false
	}
	err = h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", id, org.ID).
		First(&directive).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "directive not found"})
		return directive, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load directive"})
		return directive, false
	}
	return directive, true
}

// resolveDirectiveChannel parses channel_id ("", "org" => org-wide) and
// verifies a UUID target channel belongs to the org.
func (h *DirectiveHandler) resolveDirectiveChannel(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, raw *string) (*uuid.UUID, bool) {
	if raw == nil {
		return nil, true
	}
	value := strings.TrimSpace(*raw)
	if value == "" || value == "org" {
		return nil, true
	}
	channelID, err := uuid.Parse(value)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid channel_id"})
		return nil, false
	}
	var count int64
	if err := h.db.WithContext(r.Context()).
		Model(&model.Channel{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", channelID, orgID).
		Count(&count).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate channel"})
		return nil, false
	}
	if count == 0 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "channel not found"})
		return nil, false
	}
	return &channelID, true
}

func directiveToResponse(directive model.AgentDirective) directiveResponse {
	return directiveResponse{
		ID:              directive.ID.String(),
		ChannelID:       formatUUIDPtr(directive.ChannelID),
		Content:         directive.Content,
		Source:          directive.Source,
		Active:          directive.Active,
		CreatedByUserID: formatUUIDPtr(directive.CreatedByUserID),
		CreatedAt:       directive.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:       directive.UpdatedAt.UTC().Format(time.RFC3339),
	}
}
