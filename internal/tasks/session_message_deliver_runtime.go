package tasks

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

const agentSandboxStrategyAlwaysOn = "always_on"

func (h *SessionMessageDeliverHandler) loadRuntimeSandbox(ctx context.Context, session model.Session, agent *model.Agent) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("session message delivery: agent must have org_id")
	}
	selector := agentRuntimeSelector(h.db, h.compileDeps)
	if agent.SandboxStrategy == agentSandboxStrategyAlwaysOn {
		return selector.MainRuntime(ctx, *agent.OrgID, agent.ID)
	}
	if session.SandboxID == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return selector.MainRuntimeByID(ctx, *agent.OrgID, agent.ID, *session.SandboxID)
}

func (h *SessionMessageDeliverHandler) ensureRuntimeClient(ctx context.Context, session model.Session, agent *model.Agent) (*model.Sandbox, *agentruntime.Client, error) {
	if agent != nil && agent.SandboxStrategy == agentSandboxStrategyAlwaysOn {
		return h.ensureAlwaysOnRuntimeClient(ctx, session, agent)
	}
	return h.ensureRuntimeClientUnlocked(ctx, session, agent)
}

func (h *SessionMessageDeliverHandler) ensureAlwaysOnRuntimeClient(ctx context.Context, session model.Session, agent *model.Agent) (*model.Sandbox, *agentruntime.Client, error) {
	var sb *model.Sandbox
	var client *agentruntime.Client
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", sessionAgentSandboxLockKey(agent.ID)).Error; err != nil {
			return fmt.Errorf("acquire agent sandbox lock: %w", err)
		}
		var ensureErr error
		sb, client, ensureErr = h.ensureRuntimeClientUnlocked(ctx, session, agent)
		return ensureErr
	})
	return sb, client, err
}

// sessionAgentSandboxLockKey mirrors handler.agentSandboxLockKey so session
// delivery serializes with agent sync during always-on sandbox provisioning.
func sessionAgentSandboxLockKey(agentID uuid.UUID) int64 {
	return int64(binary.BigEndian.Uint64(agentID[:8])) // #nosec G115 -- hash truncation; sign bit is part of the hash distribution
}

func (h *SessionMessageDeliverHandler) ensureRuntimeClientUnlocked(ctx context.Context, session model.Session, agent *model.Agent) (*model.Sandbox, *agentruntime.Client, error) {
	if h.compileDeps.EncKey == nil {
		return nil, nil, fmt.Errorf("session message delivery: runtime encryption key is required")
	}
	if agent == nil || agent.OrgID == nil {
		return nil, nil, fmt.Errorf("session message delivery: agent must have org_id")
	}
	sb, err := h.loadRuntimeSandbox(ctx, session, agent)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !h.allowProvisioning {
			return nil, nil, ErrSessionRuntimeNotReady
		}
		secrets, prepErr := agentruntime.PrepareStartup(ctx, h.compileDeps, agent)
		if prepErr != nil {
			return nil, nil, fmt.Errorf("prepare agent runtime startup: %w", prepErr)
		}
		sb, err = h.orchestrator.CreateAgentSandbox(ctx, agent, secrets)
		if err != nil {
			return nil, nil, fmt.Errorf("create agent sandbox: %w", err)
		}
		if err := agentruntime.AttachProxyTokenToSandbox(ctx, h.compileDeps, agent, sb.ID, secrets.ProxyTokenJTI); err != nil {
			return nil, nil, fmt.Errorf("tag agent proxy token sandbox: %w", err)
		}
	} else if err != nil {
		return nil, nil, fmt.Errorf("load agent sandbox: %w", err)
	}
	client, err := h.orchestrator.GetRuntimeClient(ctx, sb)
	if err != nil {
		return nil, nil, fmt.Errorf("get runtime client: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return nil, nil, fmt.Errorf("%w: runtime readyz: %v", ErrSessionRuntimeNotReady, err)
	}
	return sb, client, nil
}
