package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"github.com/gorilla/websocket"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *RuntimeStreamIngressHandler) loadRuntimeIngressSandbox(w http.ResponseWriter, r *http.Request) (*model.Sandbox, bool) {
	sandboxID, err := uuid.Parse(chi.URLParam(r, "sandboxID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid sandbox_id"})
		return nil, false
	}
	var sb model.Sandbox
	if err := h.db.WithContext(r.Context()).Where("id = ?", sandboxID).First(&sb).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "sandbox not found"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load sandbox"})
		return nil, false
	}
	return &sb, true
}

func (h *RuntimeStreamIngressHandler) loadRuntimeIngressSession(w http.ResponseWriter, r *http.Request, sb *model.Sandbox, rawSessionID string) (*model.Session, bool) {
	sessionID, err := uuid.Parse(strings.TrimSpace(rawSessionID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid session_id"})
		return nil, false
	}
	if sb == nil || sb.OrgID == nil || sb.AgentID == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sandbox is not attached to an org and agent"})
		return nil, false
	}
	var session model.Session
	err = h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND agent_id = ?", sessionID, *sb.OrgID, *sb.AgentID).
		First(&session).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "session not found for sandbox"})
			return nil, false
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load session"})
		return nil, false
	}
	if session.SandboxID != nil && *session.SandboxID != sb.ID {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "session is attached to a different sandbox"})
		return nil, false
	}
	return &session, true
}

func routeBoundSessionID(session *model.Session) string {
	if session == nil {
		return ""
	}
	return session.ID.String()
}

func (h *RuntimeStreamIngressHandler) verifyRuntimeBearer(ctx context.Context, sb *model.Sandbox, authorization string) bool {
	if h == nil || h.encKey == nil || sb == nil {
		return false
	}
	secret, err := h.encKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "runtime stream ingress: failed to decrypt runtime secret", "sandbox_id", sb.ID, "error", err)
		return false
	}
	fields := strings.Fields(authorization)
	if len(fields) != 2 || !strings.EqualFold(fields[0], "Bearer") {
		return false
	}
	token := strings.TrimSpace(fields[1])
	if token == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(token), []byte(secret)) == 1
}

func writeRuntimeIngressJSON(conn *websocket.Conn, value any) error {
	conn.SetWriteDeadline(time.Now().Add(10 * time.Second))
	return conn.WriteJSON(value)
}
