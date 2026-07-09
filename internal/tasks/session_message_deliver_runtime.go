package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func (h *SessionMessageDeliverHandler) loadRuntimeSandbox(ctx context.Context, session model.Session, agent *model.Agent) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("session message delivery: agent must have org_id")
	}
	if session.SandboxID == nil {
		return nil, gorm.ErrRecordNotFound
	}
	return loadAgentSandboxByID(ctx, h.db, *agent.OrgID, agent.ID, *session.SandboxID)
}

func (h *SessionMessageDeliverHandler) ensureRuntimeClient(ctx context.Context, session model.Session, agent *model.Agent) (*model.Sandbox, *agentruntime.Client, error) {
	return h.ensureRuntimeClientUnlocked(ctx, session, agent)
}

func (h *SessionMessageDeliverHandler) ensureRuntimeClientUnlocked(ctx context.Context, session model.Session, agent *model.Agent) (*model.Sandbox, *agentruntime.Client, error) {
	if h.compileDeps.EncKey == nil {
		return nil, nil, fmt.Errorf("session message delivery: runtime encryption key is required")
	}
	if agent == nil || agent.OrgID == nil {
		return nil, nil, fmt.Errorf("session message delivery: agent must have org_id")
	}
	draining, err := sessionRuntimeDraining(ctx, h.db, session)
	if err != nil {
		return nil, nil, fmt.Errorf("check session runtime drain status: %w", err)
	}
	if draining {
		return nil, nil, ErrSessionRuntimeDraining
	}
	sb, err := h.loadRuntimeSandbox(ctx, session, agent)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !h.allowProvisioning {
			return nil, nil, ErrSessionRuntimeNotReady
		}
		runtimeAgent, runtimeOptions := sessionRuntimeAgent(agent, session)
		secrets, prepErr := agentruntime.PrepareStartup(ctx, h.compileDeps, runtimeAgent)
		if prepErr != nil {
			return nil, nil, fmt.Errorf("prepare agent runtime startup: %w", prepErr)
		}
		sb, err = h.orchestrator.CreateAgentSandboxWithRuntimeOptions(ctx, runtimeAgent, secrets, runtimeOptions)
		if err != nil {
			return nil, nil, fmt.Errorf("create agent sandbox: %w", err)
		}
		if err := agentruntime.AttachProxyTokenToSandbox(ctx, h.compileDeps, runtimeAgent, sb.ID, secrets.ProxyTokenJTI); err != nil {
			return nil, nil, fmt.Errorf("tag agent proxy token sandbox: %w", err)
		}
		if err := h.db.WithContext(ctx).Model(&model.Session{}).
			Where("id = ? AND org_id = ?", session.ID, session.OrgID).
			Update("sandbox_id", sb.ID).Error; err != nil {
			return nil, nil, fmt.Errorf("attach session sandbox: %w", err)
		}
	} else if err != nil {
		return nil, nil, fmt.Errorf("load agent sandbox: %w", err)
	}
	client, err := h.orchestrator.GetRuntimeClient(ctx, sb)
	if err != nil {
		return nil, nil, fmt.Errorf("get runtime client: %w", err)
	}
	return sb, client, nil
}

func sessionRuntimeAgent(agent *model.Agent, session model.Session) (*model.Agent, agentruntime.RuntimeConfigOptions) {
	opts := agentruntime.RuntimeConfigOptions{}
	if agent == nil {
		return agent, opts
	}
	runtimeAgent := *agent
	if modelID := strings.TrimSpace(session.Model); modelID != "" {
		runtimeAgent.Model = modelID
		opts.ModelID = modelID
	}
	opts.ReasoningEffort = strings.TrimSpace(session.ReasoningEffort)
	opts.ChannelID = session.ChannelID
	return &runtimeAgent, opts
}

func sessionRuntimeDraining(ctx context.Context, db *gorm.DB, session model.Session) (bool, error) {
	if db == nil || session.OrgID == uuid.Nil || session.AgentID == uuid.Nil {
		return false, nil
	}
	if session.SandboxID != nil {
		var count int64
		if err := db.WithContext(ctx).Model(&model.Sandbox{}).
			Where("id = ? AND org_id = ? AND agent_id = ? AND status = ?", *session.SandboxID, session.OrgID, session.AgentID, string(sandbox.StatusDraining)).
			Count(&count).Error; err != nil {
			return false, err
		}
		return count > 0, nil
	}
	return false, nil
}
