package handler

import (
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *MemoryHandler) memoryRequestContext(w http.ResponseWriter, r *http.Request) (*model.Org, *uuid.UUID, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, nil, false
	}
	userID, _ := currentRequestUserID(r.Context())
	return org, userID, true
}

// memoryListScope resolves the ?channel_id filter: absent = all channels, "org"
// = org-wide only, a UUID = that channel.
func memoryListScope(r *http.Request) (memory.ChannelScope, bool) {
	raw := strings.TrimSpace(r.URL.Query().Get("channel_id"))
	switch {
	case raw == "":
		return memory.ChannelScope{AllChannels: true}, true
	case raw == "org":
		return memory.ChannelScope{}, true
	default:
		id, err := uuid.Parse(raw)
		if err != nil {
			return memory.ChannelScope{}, false
		}
		return memory.ChannelScope{ChannelID: &id}, true
	}
}

func (h *MemoryHandler) authorizeMemoryMutation(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, userID *uuid.UUID, id uuid.UUID) (model.AgentMemory, bool) {
	var mem model.AgentMemory
	err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", id, orgID).
		First(&mem).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "memory not found"})
		return mem, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load memory"})
		return mem, false
	}
	if isAPIKeyRequest(r.Context()) || h.canManageOrgMemory(r, orgID, userID) {
		return mem, true
	}
	writeJSON(w, http.StatusForbidden, errorResponse{Error: "memory access denied"})
	return mem, false
}

func (h *MemoryHandler) canManageOrgMemory(r *http.Request, orgID uuid.UUID, userID *uuid.UUID) bool {
	if isAPIKeyRequest(r.Context()) {
		return true
	}
	if userID == nil {
		return false
	}
	var membership model.OrgMembership
	if err := h.db.WithContext(r.Context()).
		Where("org_id = ? AND user_id = ?", orgID, *userID).
		First(&membership).Error; err != nil {
		return false
	}
	return membership.Role == "owner" || membership.Role == "admin"
}

func memoryIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid memory id"})
		return uuid.Nil, false
	}
	return id, true
}

func parseOptionalUUIDString(w http.ResponseWriter, raw *string, field string) (*uuid.UUID, bool) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		return nil, true
	}
	return parseRequiredUUIDString(w, raw, field)
}

func parseRequiredUUIDString(w http.ResponseWriter, raw *string, field string) (*uuid.UUID, bool) {
	if raw == nil || strings.TrimSpace(*raw) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: field + " is required"})
		return nil, false
	}
	id, err := uuid.Parse(strings.TrimSpace(*raw))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid " + field})
		return nil, false
	}
	return &id, true
}

func parseOptionalUUIDQuery(w http.ResponseWriter, r *http.Request, key string) (*uuid.UUID, bool) {
	value := strings.TrimSpace(r.URL.Query().Get(key))
	if value == "" {
		return nil, true
	}
	return parseRequiredUUIDString(w, &value, key)
}

func parseMemoryLimit(raw string, fallback int) int {
	limit, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || limit <= 0 {
		return fallback
	}
	if limit > 100 {
		return 100
	}
	return limit
}

func splitTags(raw string) []string {
	if strings.TrimSpace(raw) == "" {
		return nil
	}
	return strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
}

func memoryToResponse(mem model.AgentMemory, similarity *float64) memoryResponse {
	return memoryResponse{
		ID:                mem.ID.String(),
		OrgID:             mem.OrgID.String(),
		ChannelID:         formatUUIDPtr(mem.ChannelID),
		Content:           mem.Content,
		Tags:              []string(mem.Tags),
		Metadata:          mem.Metadata,
		EmbeddingModel:    mem.EmbeddingModel,
		EmbeddingStatus:   mem.EmbeddingStatus,
		EmbeddingRevision: mem.EmbeddingRevision,
		EmbeddingError:    mem.EmbeddingError,
		EmbeddedAt:        formatTimePtr(mem.EmbeddedAt),
		ArchivedAt:        formatTimePtr(mem.ArchivedAt),
		CreatedAt:         mem.CreatedAt.UTC().Format(time.RFC3339),
		UpdatedAt:         mem.UpdatedAt.UTC().Format(time.RFC3339),
		Similarity:        similarity,
	}
}

func writeMemoryError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "memory not found"})
	case strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "must be"):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	case strings.Contains(err.Error(), "embed") || strings.Contains(err.Error(), "OpenRouter"):
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: err.Error()})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "memory operation failed"})
	}
}
