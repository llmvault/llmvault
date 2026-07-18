package handler

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// SandboxHandler manages sandbox lifecycle via the API.
type SandboxHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

func NewSandboxHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *SandboxHandler {
	return &SandboxHandler{db: db, orchestrator: orchestrator}
}

type sandboxResponse struct {
	ID           string            `json:"id"`
	Status       string            `json:"status"`
	ExternalID   string            `json:"external_id"`
	AgentID      *string           `json:"agent_id,omitempty"`
	ExposedPorts []int             `json:"exposed_ports"`
	PreviewURLs  map[string]string `json:"preview_urls,omitempty"`
	ErrorMessage *string           `json:"error_message,omitempty"`
	LastActiveAt *string           `json:"last_active_at,omitempty"`
	CreatedAt    string            `json:"created_at"`
}

func toSandboxResponse(s model.Sandbox) sandboxResponse {
	exposedPorts, err := model.NormalizeSandboxExposedPorts(model.SandboxExposedPortsFromInt64Array(s.ExposedPorts))
	if err != nil {
		exposedPorts = model.DefaultSandboxExposedPorts()
	}
	resp := sandboxResponse{
		ID:           s.ID.String(),
		Status:       s.Status,
		ExternalID:   s.ExternalID,
		ExposedPorts: exposedPorts,
		PreviewURLs:  sandboxPreviewURLs(s, exposedPorts),
		ErrorMessage: s.ErrorMessage,
		CreatedAt:    s.CreatedAt.Format(time.RFC3339),
	}
	if s.AgentID != nil {
		id := s.AgentID.String()
		resp.AgentID = &id
	}
	if s.LastActiveAt != nil {
		t := s.LastActiveAt.Format(time.RFC3339)
		resp.LastActiveAt = &t
	}
	return resp
}

func sandboxPreviewURLs(s model.Sandbox, ports []int) map[string]string {
	if !strings.EqualFold(strings.TrimSpace(s.ProviderID), sandbox.ProviderMicrosandbox) {
		return nil
	}
	sandboxID := strings.TrimSpace(s.ExternalID)
	baseDomain := previewBaseDomainFromRuntimeURL(s.RuntimeURL, sandboxID)
	if sandboxID == "" || baseDomain == "" || len(ports) == 0 {
		return nil
	}
	out := make(map[string]string, len(ports))
	for _, port := range ports {
		out[strconv.Itoa(port)] = fmt.Sprintf("https://%d-%s.%s", port, sandboxID, baseDomain)
	}
	return out
}

func previewBaseDomainFromRuntimeURL(rawURL, sandboxID string) string {
	rawURL = strings.TrimSpace(rawURL)
	sandboxID = strings.TrimSpace(sandboxID)
	if rawURL == "" || sandboxID == "" {
		return ""
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Hostname() == "" {
		parsed, err = url.Parse("//" + rawURL)
	}
	if err != nil {
		return ""
	}
	host := strings.TrimSpace(parsed.Hostname())
	if host == "" || strings.EqualFold(host, "localhost") || net.ParseIP(host) != nil {
		return ""
	}
	marker := "-" + sandboxID + "."
	idx := strings.Index(host, marker)
	if idx < 0 {
		return ""
	}
	return strings.Trim(host[idx+len(marker):], ".")
}

// List handles GET /v1/sandboxes.
// @Summary List sandboxes
// @Description Returns sandboxes for the current organization.
// @Tags sandboxes
// @Produce json
// @Param status query string false "Filter by status (running, stopped, error)"
// @Param limit query int false "Page size"
// @Param cursor query string false "Pagination cursor"
// @Success 200 {object} paginatedResponse[sandboxResponse]
// @Security BearerAuth
// @Router /v1/sandboxes [get]
func (h *SandboxHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	limit, cursor, err := parsePagination(r)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
		return
	}

	// A sandbox embeds its agent id and preview URLs. Sandboxes are only linked
	// to an agent (no session/channel column), so scope non-managers to
	// sandboxes whose agent is visible to them; this also hides agent-less
	// sandboxes (e.g. app sandboxes) from members. Managers and API-key callers
	// see the whole org.
	orgWide, userID, err := actorSeesOrgWide(r.Context(), h.db, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve access"})
		return
	}
	q := h.db.Where("org_id = ?", org.ID)
	if !orgWide {
		q = q.Where("agent_id IN (SELECT id FROM agents WHERE team_id IN (?))", visibleTeamSubquery(h.db, userID))
	}
	if status := r.URL.Query().Get("status"); status != "" {
		q = q.Where("status = ?", status)
	}
	q = applyPagination(q, cursor, limit)

	var sandboxes []model.Sandbox
	if err := q.Find(&sandboxes).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to list sandboxes"})
		return
	}

	hasMore := len(sandboxes) > limit
	if hasMore {
		sandboxes = sandboxes[:limit]
	}

	resp := make([]sandboxResponse, len(sandboxes))
	for i, s := range sandboxes {
		resp[i] = toSandboxResponse(s)
	}

	result := paginatedResponse[sandboxResponse]{Data: resp, HasMore: hasMore}
	if hasMore {
		last := sandboxes[len(sandboxes)-1]
		c := encodeCursor(last.CreatedAt, last.ID)
		result.NextCursor = &c
	}
	writeJSON(w, http.StatusOK, result)
}

// Get handles GET /v1/sandboxes/{id}.
// @Summary Get a sandbox
// @Description Returns sandbox details by ID.
// @Tags sandboxes
// @Produce json
// @Param id path string true "Sandbox ID"
// @Success 200 {object} sandboxResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sandboxes/{id} [get]
func (h *SandboxHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing org context"})
		return
	}

	orgWide, userID, err := actorSeesOrgWide(r.Context(), h.db, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to resolve access"})
		return
	}
	id := chi.URLParam(r, "id")
	q := h.db.Where("id = ? AND org_id = ?", id, org.ID)
	if !orgWide {
		// Hidden-agent (or agent-less) sandboxes are indistinguishable from a
		// nonexistent one for a member: 404.
		q = q.Where("agent_id IN (SELECT id FROM agents WHERE team_id IN (?))", visibleTeamSubquery(h.db, userID))
	}
	var sb model.Sandbox
	if err := q.First(&sb).Error; err != nil {
		if err == gorm.ErrRecordNotFound {
			writeJSON(w, http.StatusNotFound, map[string]string{"error": "sandbox not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to get sandbox"})
		return
	}

	writeJSON(w, http.StatusOK, toSandboxResponse(sb))
}
