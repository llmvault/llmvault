package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/memory"
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
	Scope    string      `json:"scope,omitempty"`
	AgentID  *string     `json:"agent_id,omitempty"`
	UserID   *string     `json:"user_id,omitempty"`
	Content  *string     `json:"content,omitempty"`
	Tags     []string    `json:"tags,omitempty"`
	Metadata *model.JSON `json:"metadata,omitempty"`
}

type memoryResponse struct {
	ID                string     `json:"id"`
	Scope             string     `json:"scope"`
	OrgID             string     `json:"org_id"`
	AgentID           *string    `json:"agent_id,omitempty"`
	UserID            *string    `json:"user_id,omitempty"`
	Content           string     `json:"content"`
	Tags              []string   `json:"tags"`
	Metadata          model.JSON `json:"metadata"`
	EmbeddingModel    string     `json:"embedding_model"`
	EmbeddingStatus   string     `json:"embedding_status"`
	EmbeddingRevision int        `json:"embedding_revision"`
	EmbeddingError    string     `json:"embedding_error,omitempty"`
	EmbeddedAt        *string    `json:"embedded_at,omitempty"`
	ArchivedAt        *string    `json:"archived_at,omitempty"`
	CreatedAt         string     `json:"created_at"`
	UpdatedAt         string     `json:"updated_at"`
	Similarity        *float64   `json:"similarity,omitempty"`
}

type memoryListResponse struct {
	Data []memoryResponse `json:"data"`
}

func (h *MemoryHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := h.memoryRequestContext(w, r)
	if !ok {
		return
	}
	var req memoryMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	scope := strings.TrimSpace(req.Scope)
	if scope == "" {
		scope = model.AgentMemoryScopeUser
	}
	targetUser, ok := h.memoryTargetUser(w, r, scope, req.UserID, userID)
	if !ok {
		return
	}
	if scope == model.AgentMemoryScopeOrg && !h.canManageOrgMemory(r, org.ID, userID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "admin role required for org memories"})
		return
	}
	agentID, ok := parseOptionalUUIDString(w, req.AgentID, "agent_id")
	if !ok {
		return
	}
	content := ""
	if req.Content != nil {
		content = *req.Content
	}
	var metadata model.JSON
	if req.Metadata != nil {
		metadata = *req.Metadata
	}
	mem, err := h.service().Create(r.Context(), memory.CreateRequest{
		OrgID:           org.ID,
		AgentID:         agentID,
		UserID:          targetUser,
		Scope:           scope,
		Content:         content,
		Tags:            req.Tags,
		Metadata:        metadata,
		CreatedByUserID: userID,
	})
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, map[string]memoryResponse{"memory": memoryToResponse(*mem, nil)})
}

func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := h.memoryRequestContext(w, r)
	if !ok {
		return
	}
	agentID, ok := parseOptionalUUIDQuery(w, r, "agent_id")
	if !ok {
		return
	}
	limit := parseMemoryLimit(r.URL.Query().Get("limit"), 50)
	if query := strings.TrimSpace(r.URL.Query().Get("q")); query != "" {
		h.search(w, r, org.ID, userID, agentID, query, limit)
		return
	}
	rows, err := h.service().List(r.Context(), memory.ListRequest{
		OrgID:   org.ID,
		UserID:  userID,
		AgentID: agentID,
		Scope:   strings.TrimSpace(r.URL.Query().Get("scope")),
		Tags:    splitTags(r.URL.Query().Get("tags")),
		Limit:   limit,
	})
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	out := make([]memoryResponse, len(rows))
	for i := range rows {
		out[i] = memoryToResponse(rows[i], nil)
	}
	writeJSON(w, http.StatusOK, memoryListResponse{Data: out})
}

func (h *MemoryHandler) Update(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := h.memoryRequestContext(w, r)
	if !ok {
		return
	}
	id, ok := memoryIDFromRequest(w, r)
	if !ok {
		return
	}
	existing, ok := h.authorizeMemoryMutation(w, r, org.ID, userID, id)
	if !ok {
		return
	}
	var req memoryMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	agentID, ok := parseOptionalUUIDString(w, req.AgentID, "agent_id")
	if !ok {
		return
	}
	if req.AgentID == nil {
		agentID = existing.AgentID
	}
	var tags *[]string
	if req.Tags != nil {
		tags = &req.Tags
	}
	mem, err := h.service().Update(r.Context(), memory.UpdateRequest{
		OrgID:    org.ID,
		ID:       id,
		AgentID:  agentID,
		Content:  req.Content,
		Tags:     tags,
		Metadata: req.Metadata,
	})
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]memoryResponse{"memory": memoryToResponse(*mem, nil)})
}

func (h *MemoryHandler) Archive(w http.ResponseWriter, r *http.Request) {
	org, userID, ok := h.memoryRequestContext(w, r)
	if !ok {
		return
	}
	id, ok := memoryIDFromRequest(w, r)
	if !ok {
		return
	}
	if _, ok := h.authorizeMemoryMutation(w, r, org.ID, userID, id); !ok {
		return
	}
	if err := h.service().Archive(r.Context(), memory.ArchiveRequest{OrgID: org.ID, ID: id}); err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

func (h *MemoryHandler) search(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, userID, agentID *uuid.UUID, query string, limit int) {
	agent := uuid.Nil
	if agentID != nil {
		agent = *agentID
	}
	hits, err := h.service().Search(r.Context(), memory.SearchRequest{
		OrgID:   orgID,
		UserID:  userID,
		AgentID: agent,
		Query:   query,
		Tags:    splitTags(r.URL.Query().Get("tags")),
		Limit:   limit,
	})
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	out := make([]memoryResponse, len(hits))
	for i := range hits {
		similarity := hits[i].Similarity
		out[i] = memoryToResponse(hits[i].Memory, &similarity)
	}
	writeJSON(w, http.StatusOK, memoryListResponse{Data: out})
}

func (h *MemoryHandler) service() *memory.Service {
	cfg := memory.Config{
		DB:           h.db,
		CacheManager: h.cacheManager,
		EnqueueEmbed: func(ctx context.Context, id uuid.UUID, revision int) error {
			return tasks.EnqueueMemoryEmbed(ctx, h.enq, id, revision)
		},
	}
	if h.cfg != nil {
		cfg.EmbeddingModel = h.cfg.MemoryEmbeddingModel
		cfg.EmbeddingDim = h.cfg.MemoryEmbeddingDim
	}
	return memory.NewService(cfg)
}
