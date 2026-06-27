package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

var errSessionSandboxUnavailable = errors.New("session sandbox is not available yet")

func (h *SessionHandler) loadSessionSandbox(ctx context.Context, session *model.Session) (*model.Sandbox, error) {
	if session == nil {
		return nil, errSessionSandboxUnavailable
	}
	if session.SandboxID != nil {
		sb, found, err := h.loadSandboxByID(ctx, *session.SandboxID, session.OrgID)
		if err != nil {
			return nil, err
		}
		if found {
			return sb, nil
		}
	}
	return nil, errSessionSandboxUnavailable
}

func (h *SessionHandler) loadSandboxByID(ctx context.Context, id, orgID uuid.UUID) (*model.Sandbox, bool, error) {
	var sb model.Sandbox
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ?", id, orgID).
		First(&sb).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load session sandbox: %w", err)
	}
	return &sb, true, nil
}

func (h *SessionHandler) writeSessionSandboxUnavailableError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errSessionSandboxUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session sandbox is not available yet"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
	}
}
