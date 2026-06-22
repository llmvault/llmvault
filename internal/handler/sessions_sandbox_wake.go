package handler

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentsandbox"
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
	sb, err := h.loadAlwaysOnSessionSandbox(ctx, session)
	if err != nil {
		return nil, err
	}
	if sb == nil {
		return nil, errSessionSandboxUnavailable
	}
	return sb, nil
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

func (h *SessionHandler) loadAlwaysOnSessionSandbox(ctx context.Context, session *model.Session) (*model.Sandbox, error) {
	var agent model.Agent
	err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ?", session.AgentID, session.OrgID).
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load session agent: %w", err)
	}
	if agent.SandboxStrategy != agentStrategyAlwaysOn {
		return nil, nil
	}
	sb, err := agentsandbox.Selector{DB: h.db}.MainRuntime(ctx, session.OrgID, agent.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("load always-on sandbox: %w", err)
	}
	if err := h.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ? AND org_id = ?", session.ID, session.OrgID).
		Update("sandbox_id", sb.ID).Error; err != nil {
		return nil, fmt.Errorf("attach session sandbox: %w", err)
	}
	session.SandboxID = &sb.ID
	return sb, nil
}

func (h *SessionHandler) writeSessionSandboxUnavailableError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, errSessionSandboxUnavailable):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "session sandbox is not available yet"})
	default:
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
	}
}
