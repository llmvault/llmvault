package handler

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *SessionHandler) provisionPerSessionSandbox(ctx context.Context, agent *model.Agent) (*model.Sandbox, error) {
	if h == nil || h.orchestrator == nil {
		return nil, fmt.Errorf("session sandbox provisioning is not configured")
	}
	if h.compileDeps.EncKey == nil {
		return nil, fmt.Errorf("session runtime encryption key is not configured")
	}
	secrets, err := agentruntime.PrepareStartup(ctx, h.compileDeps, agent)
	if err != nil {
		return nil, fmt.Errorf("prepare agent runtime startup: %w", err)
	}
	sb, err := h.orchestrator.CreateAgentSandbox(ctx, agent, secrets)
	if err != nil {
		h.revokePerSessionStartupToken(ctx, agent, secrets.ProxyTokenJTI)
		return nil, fmt.Errorf("create agent sandbox: %w", err)
	}
	if err := agentruntime.AttachProxyTokenToSandbox(ctx, h.compileDeps, agent, sb.ID, secrets.ProxyTokenJTI); err != nil {
		h.cleanupPerSessionSandbox(ctx, sb)
		h.revokePerSessionStartupToken(ctx, agent, secrets.ProxyTokenJTI)
		return nil, fmt.Errorf("tag agent proxy token sandbox: %w", err)
	}
	return sb, nil
}

func (h *SessionHandler) cleanupFailedPerSessionCreate(ctx context.Context, sessionID uuid.UUID, sb *model.Sandbox) {
	cleanupCtx := context.WithoutCancel(ctx)
	if h != nil && h.db != nil && sessionID != uuid.Nil {
		if err := h.db.WithContext(cleanupCtx).Where("id = ?", sessionID).Delete(&model.Session{}).Error; err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("cleanup failed per-session create: delete session: %w", err), map[string]any{
				"session_id": sessionID.String(),
			})
		}
	}
	h.cleanupPerSessionSandbox(ctx, sb)
}

func (h *SessionHandler) cleanupPerSessionSandbox(ctx context.Context, sb *model.Sandbox) {
	if h == nil || h.orchestrator == nil || sb == nil {
		return
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if err := h.orchestrator.DeleteSandbox(cleanupCtx, sb); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("cleanup per-session sandbox: %w", err), map[string]any{
			"sandbox_id": sb.ID.String(),
		})
	}
	now := time.Now()
	if err := h.db.WithContext(cleanupCtx).Model(&model.Token{}).
		Where("meta ->> ? = ? AND revoked_at IS NULL", model.TokenMetaSandboxID, sb.ID.String()).
		Update("revoked_at", &now).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("cleanup per-session sandbox proxy tokens: %w", err), map[string]any{
			"sandbox_id": sb.ID.String(),
		})
	}
}

func (h *SessionHandler) revokePerSessionStartupToken(ctx context.Context, agent *model.Agent, jti string) {
	if h == nil || h.db == nil || agent == nil || agent.OrgID == nil || strings.TrimSpace(jti) == "" {
		return
	}
	now := time.Now()
	cleanupCtx := context.WithoutCancel(ctx)
	if err := h.db.WithContext(cleanupCtx).Model(&model.Token{}).
		Where("jti = ? AND org_id = ? AND revoked_at IS NULL", jti, *agent.OrgID).
		Update("revoked_at", &now).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("cleanup per-session startup proxy token: %w", err), map[string]any{
			"agent_id": agent.ID.String(),
			"jti":      jti,
		})
	}
}
