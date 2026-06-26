package handler

import (
	"net/http"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/canvasartifact"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *CanvasHandler) requireCanvasArtifactOrg(w http.ResponseWriter, r *http.Request) (*model.Org, bool) {
	if h == nil || h.artifactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas artifacts are not configured"})
		return nil, false
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, false
	}
	return org, true
}

func (h *CanvasHandler) requireCanvasArtifactRuntime(w http.ResponseWriter) bool {
	if h == nil || h.artifactSvc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas artifacts are not configured"})
		return false
	}
	return true
}

func parseCanvasArtifactFilter(w http.ResponseWriter, r *http.Request) (canvasartifact.ArtifactFilter, bool) {
	query := r.URL.Query()
	filter := canvasartifact.ArtifactFilter{
		ProjectSlug: strings.TrimSpace(query.Get("project_slug")),
	}
	if filter.ProjectSlug == "" {
		if raw := strings.TrimSpace(query.Get("project")); raw != "" {
			if id, err := uuid.Parse(raw); err == nil {
				filter.ProjectID = &id
			} else {
				filter.ProjectSlug = raw
			}
		}
	}
	if raw := strings.TrimSpace(query.Get("project_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "project_id must be a uuid"})
			return canvasartifact.ArtifactFilter{}, false
		}
		filter.ProjectID = &id
	}
	if raw := strings.TrimSpace(query.Get("session_id")); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "session_id must be a uuid"})
			return canvasartifact.ArtifactFilter{}, false
		}
		filter.SessionID = &id
	}
	return filter, true
}

func (h *CanvasHandler) previewEntryPath(r *http.Request, artifact model.CanvasArtifact) (string, error) {
	var files []model.CanvasArtifactFile
	if err := h.db.WithContext(r.Context()).
		Where("canvas_artifact_id = ? AND archived_at IS NULL", artifact.ID).
		Order("path ASC").
		Find(&files).Error; err != nil {
		return "", err
	}
	if len(files) == 0 {
		return "", gorm.ErrRecordNotFound
	}
	for _, file := range files {
		if file.Role == "entrypoint" || file.Role == "entry" {
			return file.Path, nil
		}
	}
	for _, file := range files {
		if file.Path == "index.html" || strings.HasSuffix(file.Path, "/index.html") {
			return file.Path, nil
		}
	}
	return files[0].Path, nil
}

func (h *CanvasHandler) canvasPreviewSandbox(r *http.Request, orgID, sessionID uuid.UUID) (model.Session, model.Sandbox, error) {
	var session model.Session
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", sessionID, orgID).
		First(&session).Error; err != nil {
		return model.Session{}, model.Sandbox{}, err
	}
	if session.SandboxID == nil {
		return model.Session{}, model.Sandbox{}, gorm.ErrRecordNotFound
	}
	var sb model.Sandbox
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", *session.SandboxID, orgID).
		First(&sb).Error; err != nil {
		return model.Session{}, model.Sandbox{}, err
	}
	if sb.AgentID == nil || *sb.AgentID != session.AgentID {
		return model.Session{}, model.Sandbox{}, gorm.ErrRecordNotFound
	}
	return session, sb, nil
}

func canvasPreviewToken(runtimeSecret string, session model.Session, sb model.Sandbox, expiresAt time.Time) (string, error) {
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":        "hivy",
		"aud":        "hivy-runtime",
		"org_id":     session.OrgID.String(),
		"session_id": session.ID.String(),
		"sandbox_id": sb.ID.String(),
		"scopes":     []string{"sandbox:read", "stream:read", "repo:read", "canvas:read"},
		"iat":        time.Now().UTC().Unix(),
		"nbf":        time.Now().UTC().Add(-5 * time.Second).Unix(),
		"exp":        expiresAt.Unix(),
		"jti":        uuid.NewString(),
	})
	return token.SignedString([]byte(runtimeSecret))
}
