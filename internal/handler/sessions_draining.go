package handler

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

var errSessionSandboxDraining = errors.New("agent sandbox is draining")

func (h *SessionHandler) sessionSandboxDraining(ctx context.Context, session model.Session) (bool, error) {
	if h == nil || h.db == nil || session.OrgID == uuid.Nil || session.AgentID == uuid.Nil {
		return false, nil
	}
	if session.SandboxID != nil {
		var count int64
		if err := h.db.WithContext(ctx).Model(&model.Sandbox{}).
			Where("id = ? AND org_id = ? AND agent_id = ? AND status = ?", *session.SandboxID, session.OrgID, session.AgentID, string(sandbox.StatusDraining)).
			Count(&count).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		return count > 0, nil
	}

	var running int64
	if err := h.db.WithContext(ctx).Model(&model.Sandbox{}).
		Where("org_id = ? AND agent_id = ? AND status = ?", session.OrgID, session.AgentID, string(sandbox.StatusRunning)).
		Count(&running).Error; err != nil {
		return false, err
	}
	if running > 0 {
		return false, nil
	}

	var count int64
	if err := h.db.WithContext(ctx).Model(&model.Sandbox{}).
		Where("org_id = ? AND agent_id = ? AND status = ?", session.OrgID, session.AgentID, string(sandbox.StatusDraining)).
		Count(&count).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}
	return count > 0, nil
}
