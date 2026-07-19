package handler

import (
	"context"
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
	"github.com/usehivy/hivy/internal/tasks"
)

func (h *MemoryHandler) context(w http.ResponseWriter, r *http.Request) (*model.Org, *uuid.UUID, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, nil, false
	}
	id, _ := currentRequestUserID(r.Context())
	return org, id, true
}

func (h *MemoryHandler) canManageOrg(r *http.Request, orgID uuid.UUID, userID *uuid.UUID) bool {
	if isAPIKeyRequest(r.Context()) {
		return true
	}
	role, err := orgRoleForUser(r.Context(), h.db, orgID, userID)
	return err == nil && isOrgManager(role)
}

func (h *MemoryHandler) canManageAgent(r *http.Request, orgID uuid.UUID, userID *uuid.UUID, agentID uuid.UUID) bool {
	var agent model.Agent
	if h.db.WithContext(r.Context()).Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").First(&agent).Error != nil {
		return false
	}
	return canUseTeam(r.Context(), h.db, orgID, agent.TeamID, userID, isAPIKeyRequest(r.Context()))
}

func (h *MemoryHandler) loadAuthorized(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, userID *uuid.UUID, id uuid.UUID) (model.AgentMemory, bool) {
	var mem model.AgentMemory
	if err := h.db.WithContext(r.Context()).Where("id = ? AND org_id = ? AND archived_at IS NULL", id, orgID).First(&mem).Error; err != nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "memory not found"})
		return mem, false
	}
	if !h.canManageAgent(r, orgID, userID, mem.AgentID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return mem, false
	}
	return mem, true
}

func (h *MemoryHandler) service() *memory.Service {
	cfg := memory.Config{DB: h.db, CacheManager: h.cacheManager, EnqueueEmbed: func(ctx context.Context, id uuid.UUID, revision int) error {
		return tasks.EnqueueMemoryEmbed(ctx, h.enq, id, revision)
	}}
	if h.cfg != nil {
		cfg.EmbeddingModel = h.cfg.MemoryEmbeddingModel
		cfg.EmbeddingDim = h.cfg.MemoryEmbeddingDim
	}
	return memory.NewService(cfg)
}

func memoryID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil || id == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid memory id"})
		return uuid.Nil, false
	}
	return id, true
}

func memoryLimit(raw string, fallback int) int {
	n, err := strconv.Atoi(strings.TrimSpace(raw))
	if err != nil || n < 1 {
		return fallback
	}
	if n > 100 {
		return 100
	}
	return n
}

func splitMemoryTags(raw string) []string {
	return strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ' ' || r == '\t' || r == '\n' })
}

func memoryToResponse(mem model.AgentMemory, similarity *float64) memoryResponse {
	return memoryResponse{ID: mem.ID.String(), OrgID: mem.OrgID.String(), AgentID: mem.AgentID.String(), Content: mem.Content, Tags: []string(mem.Tags), Metadata: mem.Metadata, EmbeddingStatus: mem.EmbeddingStatus, CreatedAt: mem.CreatedAt.UTC().Format(time.RFC3339), UpdatedAt: mem.UpdatedAt.UTC().Format(time.RFC3339), Similarity: similarity}
}

func writeMemoryError(w http.ResponseWriter, r *http.Request, err error) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "memory not found"})
		return
	}
	writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid memory request"})
}
