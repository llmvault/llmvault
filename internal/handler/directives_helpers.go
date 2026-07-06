package handler

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

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
