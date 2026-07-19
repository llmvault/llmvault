package handler

import (
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
type memoryMutationResponse struct {
	Memory memoryResponse `json:"memory"`
}

// Create handles POST /v1/memories. Agent memory is always owned by exactly
// one active agent; callers may manage memory for agents in teams they manage.
// @Summary Create an agent memory
// @Description Stores a memory for one active agent. Content is embedded asynchronously.
// @Tags memories
// @Accept json
// @Produce json
// @Param request body memoryMutationRequest true "Agent memory"
// @Success 201 {object} memoryMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
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
	writeJSON(w, http.StatusCreated, memoryMutationResponse{Memory: memoryToResponse(*mem, nil)})
}

// List handles GET /v1/memories?agent_id=UUID. An omitted agent_id is limited
// to org managers and is useful for administration views.
// @Summary List agent memories
// @Description Lists memories for one agent, or for every agent when agent_id is omitted by an organization manager. Pass q to semantic-search instead.
// @Tags memories
// @Produce json
// @Param agent_id query string false "Agent UUID; omit for every agent"
// @Param q query string false "Semantic search query"
// @Param tags query string false "Comma- or space-separated tag filters"
// @Param limit query int false "Max items (1-100, default 50)"
// @Success 200 {object} memoryListResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
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
// @Summary Update an agent memory
// @Description Updates a memory's content, tags, or metadata. Editing content re-embeds it.
// @Tags memories
// @Accept json
// @Produce json
// @Param id path string true "Memory UUID"
// @Param request body memoryMutationRequest true "Fields to update"
// @Success 200 {object} memoryMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
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
	writeJSON(w, http.StatusOK, memoryMutationResponse{Memory: memoryToResponse(*updated, nil)})
}

// Archive handles DELETE /v1/memories/{id}.
// @Summary Archive an agent memory
// @Description Archives a memory so the agent no longer recalls it.
// @Tags memories
// @Produce json
// @Param id path string true "Memory UUID"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
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
