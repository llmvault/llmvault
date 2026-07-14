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

func (h *SessionHandler) provisionSessionSandbox(ctx context.Context, agent *model.Agent, teamID uuid.UUID, modelID string, reasoningEffort string, mcpContext agentruntime.MCPRuntimeContext) (*model.Sandbox, error) {
	if h == nil || h.orchestrator == nil {
		return nil, fmt.Errorf("session sandbox provisioning is not configured")
	}
	if h.compileDeps.EncKey == nil {
		return nil, fmt.Errorf("session runtime encryption key is not configured")
	}
	totalStarted := time.Now()
	phaseStarted := totalStarted
	logPhase := func(phase string, attrs ...any) {
		attrs = append(attrs,
			"total_ms", time.Since(totalStarted).Milliseconds(),
		)
		logging.LogPhase(ctx, "session sandbox provision phase", phase, phaseStarted, attrs...)
		phaseStarted = time.Now()
	}
	runtimeAgent := sessionSandboxRuntimeAgent(agent, modelID)
	logPhase("start",
		"agent_id", runtimeAgent.ID,
		"org_id", runtimeAgent.OrgID,
		"model", strings.TrimSpace(runtimeAgent.Model),
		"has_reasoning_effort", strings.TrimSpace(reasoningEffort) != "",
	)
	secrets, err := agentruntime.PrepareStartup(ctx, h.compileDeps, &runtimeAgent)
	if err != nil {
		return nil, fmt.Errorf("prepare agent runtime startup: %w", err)
	}
	logPhase("prepare startup", "agent_id", runtimeAgent.ID, "org_id", runtimeAgent.OrgID, "proxy_expires_at", secrets.ProxyExpires)
	sb, err := h.orchestrator.CreateAgentSandboxWithRuntimeOptions(ctx, &runtimeAgent, secrets, agentruntime.RuntimeConfigOptions{
		ModelID:         runtimeAgent.Model,
		ReasoningEffort: reasoningEffort,
		TeamID:          teamID,
		MCPContext:      mcpContext,
	})
	if err != nil {
		h.revokeSessionStartupToken(ctx, &runtimeAgent, secrets.ProxyTokenJTI)
		return nil, fmt.Errorf("create agent sandbox: %w", err)
	}
	logPhase("create agent sandbox", "agent_id", runtimeAgent.ID, "org_id", runtimeAgent.OrgID, "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	if err := agentruntime.AttachProxyTokenToSandbox(ctx, h.compileDeps, &runtimeAgent, sb.ID, secrets.ProxyTokenJTI); err != nil {
		h.cleanupSessionSandbox(ctx, sb)
		h.revokeSessionStartupToken(ctx, &runtimeAgent, secrets.ProxyTokenJTI)
		return nil, fmt.Errorf("tag agent proxy token sandbox: %w", err)
	}
	logPhase("attach startup token", "agent_id", runtimeAgent.ID, "org_id", runtimeAgent.OrgID, "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	logPhase("complete", "agent_id", runtimeAgent.ID, "org_id", runtimeAgent.OrgID, "sandbox_id", sb.ID, "external_id", sb.ExternalID)
	return sb, nil
}

func sessionSandboxRuntimeAgent(agent *model.Agent, modelID string) model.Agent {
	if agent == nil {
		return model.Agent{}
	}
	runtimeAgent := *agent
	if modelID = strings.TrimSpace(modelID); modelID != "" {
		runtimeAgent.Model = modelID
	}
	return runtimeAgent
}

func (h *SessionHandler) cleanupFailedSessionCreate(ctx context.Context, sessionID uuid.UUID, sb *model.Sandbox) {
	cleanupCtx := context.WithoutCancel(ctx)
	if h != nil && h.db != nil && sessionID != uuid.Nil {
		if err := h.db.WithContext(cleanupCtx).Where("id = ?", sessionID).Delete(&model.Session{}).Error; err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("cleanup failed session create: delete session: %w", err), map[string]any{
				"session_id": sessionID.String(),
			})
		}
	}
	h.cleanupSessionSandbox(ctx, sb)
}

func (h *SessionHandler) cleanupSessionSandbox(ctx context.Context, sb *model.Sandbox) {
	if h == nil || h.orchestrator == nil || sb == nil {
		return
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if err := h.orchestrator.DeleteSandbox(cleanupCtx, sb); err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("cleanup session sandbox: %w", err), map[string]any{
			"sandbox_id": sb.ID.String(),
		})
	}
	now := time.Now()
	if err := h.db.WithContext(cleanupCtx).Model(&model.Token{}).
		Where("meta ->> ? = ? AND revoked_at IS NULL", model.TokenMetaSandboxID, sb.ID.String()).
		Update("revoked_at", &now).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("cleanup session sandbox proxy tokens: %w", err), map[string]any{
			"sandbox_id": sb.ID.String(),
		})
	}
}

func (h *SessionHandler) revokeSessionStartupToken(ctx context.Context, agent *model.Agent, jti string) {
	if h == nil || h.db == nil || agent == nil || agent.OrgID == nil || strings.TrimSpace(jti) == "" {
		return
	}
	now := time.Now()
	cleanupCtx := context.WithoutCancel(ctx)
	if err := h.db.WithContext(cleanupCtx).Model(&model.Token{}).
		Where("jti = ? AND org_id = ? AND revoked_at IS NULL", jti, *agent.OrgID).
		Update("revoked_at", &now).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("cleanup session startup proxy token: %w", err), map[string]any{
			"agent_id": agent.ID.String(),
			"jti":      jti,
		})
	}
}
