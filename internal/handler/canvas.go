package handler

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaspkg "github.com/usehivy/hivy/internal/canvas"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type CanvasService interface {
	SessionURLForUser(ctx context.Context, orgID, userID, canvasFileID uuid.UUID, pageID *uuid.UUID) (*canvaspkg.SessionURLResult, error)
	CreateProjectForAgent(ctx context.Context, agentID uuid.UUID, name string) (*canvaspkg.ProjectCreateResult, error)
	ListProjectsForAgent(ctx context.Context, agentID uuid.UUID) (*canvaspkg.ProjectListResult, error)
	CreateFileForAgent(ctx context.Context, agentID, projectID uuid.UUID, name string) (*canvaspkg.FileCreateResult, error)
	ListFilesForAgent(ctx context.Context, agentID uuid.UUID) (*canvaspkg.FileListResult, error)
}

type CanvasHandler struct {
	db     *gorm.DB
	encKey *crypto.SymmetricKey
	svc    CanvasService
}

type canvasSessionURLRequest struct {
	FileID string  `json:"file_id"`
	PageID *string `json:"page_id,omitempty"`
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
	if h == nil || h.svc == nil {
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
	result, err := h.svc.CreateProjectForAgent(r.Context(), agentID, req.Name)
	if err != nil {
		h.writeRuntimeCanvasError(w, r, err, "create canvas project", agentID)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (h *CanvasHandler) ListAgentProjects(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	agentID, ok := h.authorizeRuntimeRequest(w, r)
	if !ok {
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

func (h *CanvasHandler) authorizeRuntimeRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	if h == nil || h.db == nil || h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime authentication is not configured"})
		return uuid.Nil, false
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid agent_id"})
		return uuid.Nil, false
	}
	token := extractBearerToken(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing authorization"})
		return uuid.Nil, false
	}
	if err := h.verifyRuntimeSecret(r.Context(), agentID, token); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return uuid.Nil, false
		}
		if errors.Is(err, errInvalidRuntimeSecret) {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
			return uuid.Nil, false
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "canvas runtime auth", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to verify credentials"})
		return uuid.Nil, false
	}
	return agentID, true
}

var errInvalidRuntimeSecret = errors.New("invalid runtime secret")

func (h *CanvasHandler) verifyRuntimeSecret(ctx context.Context, agentID uuid.UUID, bearerToken string) error {
	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return err
	}
	var sandboxes []model.Sandbox
	if err := h.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&sandboxes).Error; err != nil {
		return fmt.Errorf("load sandboxes: %w", err)
	}
	for _, sb := range sandboxes {
		decrypted, err := h.encKey.DecryptString(sb.EncryptedRuntimeSecret)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(bearerToken), []byte(decrypted)) == 1 {
			return nil
		}
	}
	return errInvalidRuntimeSecret
}

func (h *CanvasHandler) writeRuntimeCanvasError(w http.ResponseWriter, r *http.Request, err error, op string, agentID uuid.UUID) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "canvas resource not found"})
		return
	}
	if errors.Is(err, canvaspkg.ErrNotConfigured) {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	logging.FromContext(r.Context()).ErrorContext(r.Context(), "canvas runtime request failed", "operation", op, "error", err, "agent_id", agentID)
	writeJSON(w, http.StatusBadGateway, errorResponse{Error: "canvas request failed"})
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
