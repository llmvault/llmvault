package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaspkg "github.com/usehivy/hivy/internal/canvas"
	"github.com/usehivy/hivy/internal/canvasartifact"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
)

type CanvasService interface {
	SessionURLForUser(ctx context.Context, orgID, userID, canvasFileID uuid.UUID, pageID *uuid.UUID) (*canvaspkg.SessionURLResult, error)
	ListProjectCatalogForOrg(ctx context.Context, orgID uuid.UUID) (*canvaspkg.ProjectCatalogResult, error)
	CreateProjectForAgent(ctx context.Context, agentID uuid.UUID, name string) (*canvaspkg.ProjectCreateResult, error)
	ListProjectsForAgent(ctx context.Context, agentID uuid.UUID) (*canvaspkg.ProjectListResult, error)
	CreateFileForAgent(ctx context.Context, agentID, projectID uuid.UUID, name string) (*canvaspkg.FileCreateResult, error)
	ListFilesForAgent(ctx context.Context, agentID uuid.UUID) (*canvaspkg.FileListResult, error)
}

type CanvasArtifactService interface {
	CreateProjectForAgent(ctx context.Context, agentID uuid.UUID, req canvasartifact.ProjectCreateRequest) (*canvasartifact.ProjectResponse, error)
	ListProjectsForAgent(ctx context.Context, agentID uuid.UUID) (*canvasartifact.ProjectListResponse, error)
	ListProjectsForOrg(ctx context.Context, orgID uuid.UUID, sessionID *uuid.UUID) (*canvasartifact.ProjectListResponse, error)
	ListArtifactsForAgent(ctx context.Context, agentID uuid.UUID, filter canvasartifact.ArtifactFilter) (*canvasartifact.ArtifactListResponse, error)
	ListArtifactsForOrg(ctx context.Context, orgID uuid.UUID, filter canvasartifact.ArtifactFilter) (*canvasartifact.ArtifactListResponse, error)
	GetArtifactForOrg(ctx context.Context, orgID, artifactID uuid.UUID) (*canvasartifact.ArtifactResponse, error)
	SyncArtifactForAgent(ctx context.Context, agentID uuid.UUID, req canvasartifact.SyncRequest) (*canvasartifact.SyncResponse, error)
	SnapshotForAgent(ctx context.Context, agentID uuid.UUID) (*canvasartifact.SnapshotResponse, error)
}

type CanvasHandler struct {
	db          *gorm.DB
	encKey      *crypto.SymmetricKey
	svc         CanvasService
	artifactSvc CanvasArtifactService
}

type canvasSessionURLRequest struct {
	FileID string  `json:"file_id"`
	PageID *string `json:"page_id,omitempty"`
}

type canvasSessionURLResponse struct {
	URL       string     `json:"url"`
	ExpiresIn int64      `json:"expires_in"`
	FileID    uuid.UUID  `json:"file_id"`
	PageID    *uuid.UUID `json:"page_id,omitempty"`
	TeamID    uuid.UUID  `json:"team_id"`
}

type canvasProjectCreateRequest struct {
	Name string `json:"name"`
}

type canvasFileCreateRequest struct {
	Name      string `json:"name"`
	ProjectID string `json:"project_id"`
}

func NewCanvasHandler(db *gorm.DB, encKey *crypto.SymmetricKey, svc CanvasService) *CanvasHandler {
	return &CanvasHandler{db: db, encKey: encKey, svc: svc}
}

func (h *CanvasHandler) WithArtifactService(svc CanvasArtifactService) *CanvasHandler {
	h.artifactSvc = svc
	return h
}

// SessionURL handles POST /v1/canvas/session-url.
// @Summary Create Canvas session URL
// @Description Returns a short-lived Canvas iframe session URL for a file visible to the current user.
// @Tags canvas
// @Accept json
// @Produce json
// @Param body body canvasSessionURLRequest true "Canvas file target"
// @Success 200 {object} canvasSessionURLResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/canvas/session-url [post]
func (h *CanvasHandler) SessionURL(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok || claims == nil || claims.UserID == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing user context"})
		return
	}
	userID, err := uuid.Parse(claims.UserID)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid user context"})
		return
	}
	var req canvasSessionURLRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	fileID, err := uuid.Parse(strings.TrimSpace(req.FileID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "file_id must be a uuid"})
		return
	}
	pageID, ok := parseOptionalCanvasUUID(w, req.PageID, "page_id")
	if !ok {
		return
	}
	result, err := h.svc.SessionURLForUser(r.Context(), org.ID, userID, fileID, pageID)
	if err != nil {
		if errors.Is(err, canvaspkg.ErrNotConfigured) {
			writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
			return
		}
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "canvas file not found"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "canvas session url", "error", err, "org_id", org.ID, "file_id", fileID)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "failed to create canvas session"})
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *CanvasHandler) CreateAgentProject(w http.ResponseWriter, r *http.Request) {
	if h == nil || (h.svc == nil && h.artifactSvc == nil) {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
		return
	}
	var req canvasProjectCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if h.artifactSvc != nil {
		result, err := h.artifactSvc.CreateProjectForAgent(r.Context(), agentID, canvasartifact.ProjectCreateRequest{Name: req.Name})
		if err != nil {
			h.writeRuntimeCanvasError(w, r, err, "create canvas project", agentID)
			return
		}
		writeJSON(w, http.StatusCreated, result)
		return
	}
	result, err := h.svc.CreateProjectForAgent(r.Context(), agentID, req.Name)
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "create canvas project", agentID)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *CanvasHandler) ListAgentProjects(w http.ResponseWriter, r *http.Request) {
	if h == nil || (h.svc == nil && h.artifactSvc == nil) {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
		return
	}
	if h.artifactSvc != nil {
		result, err := h.artifactSvc.ListProjectsForAgent(r.Context(), agentID)
		if err != nil {
			h.writeRuntimeCanvasError(w, r, err, "list canvas projects", agentID)
			return
		}
		writeJSON(w, http.StatusOK, result)
		return
	}
	result, err := h.svc.ListProjectsForAgent(r.Context(), agentID)
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "list canvas projects", agentID)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func (h *CanvasHandler) CreateAgentFile(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
		return
	}
	var req canvasFileCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	projectID, err := uuid.Parse(strings.TrimSpace(req.ProjectID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "project_id must be a uuid"})
		return
	}
	result, err := h.svc.CreateFileForAgent(r.Context(), agentID, projectID, req.Name)
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "create canvas file", agentID)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *CanvasHandler) ListAgentFiles(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
		return
	}
	result, err := h.svc.ListFilesForAgent(r.Context(), agentID)
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "list canvas files", agentID)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

func parseOptionalCanvasUUID(w http.ResponseWriter, value *string, field string) (*uuid.UUID, bool) {
	if value == nil || strings.TrimSpace(*value) == "" {
		return nil, true
	}
	parsed, err := uuid.Parse(strings.TrimSpace(*value))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: field + " must be a uuid"})
		return nil, false
	}
	return &parsed, true
}
