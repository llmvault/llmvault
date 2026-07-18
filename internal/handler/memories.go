package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

type MemoryHandler struct {
	db           *gorm.DB
	enq          enqueue.TaskEnqueuer
	cacheManager *cache.Manager
	cfg          *config.Config
}

func NewMemoryHandler(db *gorm.DB, enq enqueue.TaskEnqueuer, cacheManager *cache.Manager, cfg *config.Config) *MemoryHandler {
	return &MemoryHandler{db: db, enq: enq, cacheManager: cacheManager, cfg: cfg}
}

type memoryMutationRequest struct {
	AgentID  string      `json:"agent_id"`
	Content  *string     `json:"content,omitempty"`
	Tags     []string    `json:"tags,omitempty"`
	Metadata *model.JSON `json:"metadata,omitempty"`
}
type memoryResponse struct {
	ID              string     `json:"id"`
	OrgID           string     `json:"org_id"`
	AgentID         string     `json:"agent_id"`
	Content         string     `json:"content"`
	Tags            []string   `json:"tags"`
	Metadata        model.JSON `json:"metadata"`
	EmbeddingStatus string     `json:"embedding_status"`
	CreatedAt       string     `json:"created_at"`
	UpdatedAt       string     `json:"updated_at"`
	Similarity      *float64   `json:"similarity,omitempty"`
}
type memoryListResponse struct {
	Data []memoryResponse `json:"data"`
}

// Create handles POST /v1/memories. Agent memory is always owned by exactly
// one active agent; callers may manage memory for agents in teams they manage.
// @Router /v1/memories [post]
func (h *MemoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := h.context(w, r)
	if !ok {
		return
	}
	var req memoryMutationRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	agentID, err := uuid.Parse(strings.TrimSpace(req.AgentID))
	if err != nil || agentID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent_id must be a uuid"})
		return
	}
	if !h.canManageAgent(r, org.ID, userID, agentID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return
	}
	content := ""
	if req.Content != nil {
		content = *req.Content
	}
	metadata := model.JSON{}
	if req.Metadata != nil {
		metadata = *req.Metadata
	}
	mem, err := h.service().Create(r.Context(), memory.CreateRequest{OrgID: org.ID, AgentID: agentID, Content: content, Tags: req.Tags, Metadata: metadata, CreatedByUserID: userID})
	if err != nil {
		writeMemoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]memoryResponse{"memory": memoryToResponse(*mem, nil)})
}

// List handles GET /v1/memories?agent_id=UUID. An omitted agent_id is limited
// to org managers and is useful for administration views.
// @Router /v1/memories [get]
func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := h.context(w, r)
	if !ok {
		return
	}
	scope := memory.AgentScope{}
	if raw := strings.TrimSpace(r.URL.Query().Get("agent_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil || id == uuid.Nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "agent_id must be a uuid"})
			return
		}
		if !h.canManageAgent(r, org.ID, userID, id) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		scope.AgentID = id
	} else {
		if !h.canManageOrg(r, org.ID, userID) {
			writeJSON(w, http.StatusForbidden, errorResponse{Error: "admin role required"})
			return
		}
		scope.AllAgents = true
	}
	limit := memoryLimit(r.URL.Query().Get("limit"), 50)
	tags := splitMemoryTags(r.URL.Query().Get("tags"))
	query := strings.TrimSpace(r.URL.Query().Get("q"))
	if query != "" {
		hits, err := h.service().Search(r.Context(), memory.SearchRequest{OrgID: org.ID, Scope: scope, Query: query, Tags: tags, Limit: limit})
		if err != nil {
			writeMemoryError(w, r, err)
			return
		}
		out := make([]memoryResponse, len(hits))
		for i := range hits {
			score := hits[i].Similarity
			out[i] = memoryToResponse(hits[i].Memory, &score)
		}
		writeJSON(w, http.StatusOK, memoryListResponse{Data: out})
		return
	}
	rows, err := h.service().List(r.Context(), memory.ListRequest{OrgID: org.ID, Scope: scope, Tags: tags, Limit: limit})
	if err != nil {
		writeMemoryError(w, r, err)
		return
	}
	out := make([]memoryResponse, len(rows))
	for i := range rows {
		out[i] = memoryToResponse(rows[i], nil)
	}
	writeJSON(w, http.StatusOK, memoryListResponse{Data: out})
}

// Update handles PATCH /v1/memories/{id}.
// @Router /v1/memories/{id} [patch]
func (h *MemoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := h.context(w, r)
	if !ok {
		return
	}
	id, ok := memoryID(w, r)
	if !ok {
		return
	}
	mem, ok := h.loadAuthorized(w, r, org.ID, userID, id)
	if !ok {
		return
	}
	var req memoryMutationRequest
	if json.NewDecoder(r.Body).Decode(&req) != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	var tags *[]string
	if req.Tags != nil {
		tags = &req.Tags
	}
	updated, err := h.service().Update(r.Context(), memory.UpdateRequest{OrgID: org.ID, ID: mem.ID, Content: req.Content, Tags: tags, Metadata: req.Metadata})
	if err != nil {
		writeMemoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]memoryResponse{"memory": memoryToResponse(*updated, nil)})
}

// Archive handles DELETE /v1/memories/{id}.
// @Router /v1/memories/{id} [delete]
func (h *MemoryHandler) Archive(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := h.context(w, r)
	if !ok {
		return
	}
	id, ok := memoryID(w, r)
	if !ok {
		return
	}
	_, ok = h.loadAuthorized(w, r, org.ID, userID, id)
	if !ok {
		return
	}
	if err := h.service().Archive(r.Context(), memory.ArchiveRequest{OrgID: org.ID, ID: id}); err != nil {
		writeMemoryError(w, r, err)
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "archived"})
}

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
