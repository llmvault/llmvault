package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/canvasartifact"
	"github.com/usehivy/hivy/internal/crypto"
)

type CanvasArtifactService interface {
	CreateProjectForAgent(ctx context.Context, agentID uuid.UUID, req canvasartifact.ProjectCreateRequest) (*canvasartifact.ProjectResponse, error)
	ListProjectsForAgent(ctx context.Context, agentID uuid.UUID) (*canvasartifact.ProjectListResponse, error)
	ListProjectsForOrg(ctx context.Context, orgID uuid.UUID, sessionID *uuid.UUID, visibleSessions *gorm.DB) (*canvasartifact.ProjectListResponse, error)
	ListArtifactsForAgent(ctx context.Context, agentID uuid.UUID, filter canvasartifact.ArtifactFilter) (*canvasartifact.ArtifactListResponse, error)
	ListArtifactsForOrg(ctx context.Context, orgID uuid.UUID, filter canvasartifact.ArtifactFilter) (*canvasartifact.ArtifactListResponse, error)
	GetArtifactForOrg(ctx context.Context, orgID, artifactID uuid.UUID, visibleSessions *gorm.DB) (*canvasartifact.ArtifactResponse, error)
	SyncArtifactForAgent(ctx context.Context, agentID uuid.UUID, req canvasartifact.SyncRequest) (*canvasartifact.SyncResponse, error)
	SnapshotForAgent(ctx context.Context, agentID uuid.UUID) (*canvasartifact.SnapshotResponse, error)
}

type CanvasHandler struct {
	db          *gorm.DB
	encKey      *crypto.SymmetricKey
	artifactSvc CanvasArtifactService
}

type canvasProjectCreateRequest struct {
	Name string `json:"name"`
}

func NewCanvasHandler(db *gorm.DB, encKey *crypto.SymmetricKey) *CanvasHandler {
	return &CanvasHandler{db: db, encKey: encKey}
}

func (h *CanvasHandler) WithArtifactService(svc CanvasArtifactService) *CanvasHandler {
	h.artifactSvc = svc
	return h
}

// visibleSessionScope returns the subquery of sessions a non-manager member may
// view (used to scope canvas artifacts by their source session), or nil for an
// org-wide caller (manager or API-key).
func (h *CanvasHandler) visibleSessionScope(ctx context.Context, orgID uuid.UUID) (*gorm.DB, error) {
	orgWide, userID, err := actorSeesOrgWide(ctx, h.db, orgID)
	if err != nil {
		return nil, err
	}
	if orgWide {
		return nil, nil
	}
	return visibleSessionIDSubquery(h.db, orgID, userID), nil
}

func (h *CanvasHandler) CreateAgentProject(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.artifactSvc == nil {
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
	result, err := h.artifactSvc.CreateProjectForAgent(r.Context(), agentID, canvasartifact.ProjectCreateRequest{Name: req.Name})
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "create canvas project", agentID)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *CanvasHandler) ListAgentProjects(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.artifactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
		return
	}
	result, err := h.artifactSvc.ListProjectsForAgent(r.Context(), agentID)
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "list canvas projects", agentID)
		return
	}
	writeJSON(w, http.StatusOK, result)
}
