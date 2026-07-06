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
	ChannelID *string     `json:"channel_id,omitempty"`
	Content   *string     `json:"content,omitempty"`
	Tags      []string    `json:"tags,omitempty"`
	Metadata  *model.JSON `json:"metadata,omitempty"`
}

type memoryResponse struct {
	ID                string     `json:"id"`
	OrgID             string     `json:"org_id"`
	ChannelID         *string    `json:"channel_id,omitempty"`
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

// Create handles POST /v1/memories.
// @Summary Create a memory
// @Description Stores a memory in a channel (channel_id) or organization-wide (omit channel_id). Requires an org admin/owner. Content is embedded asynchronously.
// @Tags memories
// @Accept json
// @Produce json
// @Param request body memoryMutationRequest true "Memory content, optional channel_id and tags"
// @Success 201 {object} map[string]memoryResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Security BearerAuth
// @Router /v1/memories [post]
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
	// Writing memories is an org-admin action (channel-classified in the future
	// settings UI); API keys with the memories scope are also allowed.
	if !h.canManageOrgMemory(r, org.ID, userID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "admin role required to manage memories"})
		return
	}
	channelID, ok := parseOptionalUUIDString(w, req.ChannelID, "channel_id")
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
		ChannelID:       channelID,
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

// List handles GET /v1/memories.
// @Summary List memories
// @Description Lists memories, optionally filtered by channel_id ("org" for organization-wide, a UUID for one channel, omitted for all). Pass q to semantic-search instead.
// @Tags memories
// @Produce json
// @Param channel_id query string false "Filter: channel UUID, \"org\", or omitted for all"
// @Param q query string false "Semantic search query"
// @Param tags query string false "Comma-separated tag filters"
// @Param limit query int false "Max items (1-100, default 50)"
// @Success 200 {object} memoryListResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/memories [get]
func (h *MemoryHandler) List(w http.ResponseWriter, r *http.Request) {
	org, _, ok := h.memoryRequestContext(w, r)
	if !ok {
		return
	}
	scope, ok := memoryListScope(r)
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid channel_id"})
		return
	}
	limit := parseMemoryLimit(r.URL.Query().Get("limit"), 50)
	if query := strings.TrimSpace(r.URL.Query().Get("q")); query != "" {
		h.search(w, r, org.ID, scope, query, limit)
		return
	}
	rows, err := h.service().List(r.Context(), memory.ListRequest{
		OrgID: org.ID,
		Scope: scope,
		Tags:  splitTags(r.URL.Query().Get("tags")),
		Limit: limit,
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

// Update handles PATCH /v1/memories/{id}.
// @Summary Update a memory
// @Description Updates a memory's content, tags, or metadata. Editing content re-embeds it. Requires an org admin/owner.
// @Tags memories
// @Accept json
// @Produce json
// @Param id path string true "Memory UUID"
// @Param request body memoryMutationRequest true "Fields to update"
// @Success 200 {object} map[string]memoryResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/memories/{id} [patch]
func (h *MemoryHandler) Update(w http.ResponseWriter, r *http.Request) {
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
	var req memoryMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	var tags *[]string
	if req.Tags != nil {
		tags = &req.Tags
	}
	updateReq := memory.UpdateRequest{
		OrgID:    org.ID,
		ID:       id,
		Content:  req.Content,
		Tags:     tags,
		Metadata: req.Metadata,
	}
	// A present channel_id re-scopes the memory: "org"/"" moves it org-wide, a
	// UUID moves it to that channel. Absent leaves the channel unchanged.
	if req.ChannelID != nil {
		updateReq.UpdateChannel = true
		if raw := strings.TrimSpace(*req.ChannelID); raw != "" && raw != "org" {
			channelID, ok := parseRequiredUUIDString(w, req.ChannelID, "channel_id")
			if !ok {
				return
			}
			updateReq.ChannelID = channelID
		}
	}
	mem, err := h.service().Update(r.Context(), updateReq)
	if err != nil {
		writeMemoryError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]memoryResponse{"memory": memoryToResponse(*mem, nil)})
}

// Archive handles DELETE /v1/memories/{id}.
// @Summary Forget a memory
// @Description Archives a memory so agents no longer recall it. Requires an org admin/owner.
// @Tags memories
// @Produce json
// @Param id path string true "Memory UUID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/memories/{id} [delete]
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

func (h *MemoryHandler) search(w http.ResponseWriter, r *http.Request, orgID uuid.UUID, scope memory.ChannelScope, query string, limit int) {
	hits, err := h.service().Search(r.Context(), memory.SearchRequest{
		OrgID: orgID,
		Scope: scope,
		Query: query,
		Tags:  splitTags(r.URL.Query().Get("tags")),
		Limit: limit,
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
