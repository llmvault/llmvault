package handler

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/agentsandbox"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
	"gorm.io/gorm"
)

func (h *AgentHandler) ensureAgentSandbox(ctx context.Context, agent *model.Agent) (*model.Sandbox, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent must have org_id")
	}
	// Concurrent syncs (org PATCH, env-var writes, webhooks) all reach here during
	// org bootstrap; without serialisation each runs check-then-create and provisions
	// its own billing sandbox. A per-agent advisory lock lets only one provision;
	// the others block, then reuse the winner's row.
	var sb *model.Sandbox
	txErr := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", agentSandboxLockKey(agent.ID)).Error; err != nil {
			return fmt.Errorf("acquire agent sandbox lock: %w", err)
		}
		ensured, err := h.ensureAgentSandboxLocked(ctx, agent)
		if err != nil {
			return err
		}
		sb = ensured
		return nil
	})
	if txErr != nil {
		return nil, txErr
	}
	return sb, nil
}

func (h *AgentHandler) ensureAgentSandboxLocked(ctx context.Context, agent *model.Agent) (*model.Sandbox, error) {
	sb, err := h.mainAgentRuntimeSelector().MainRuntime(ctx, *agent.OrgID, agent.ID)
	if err == nil {
		// The selector also returns creating/starting/stopped rows; route them
		// through EnsureSandboxActive so an in-flight/idle sandbox is reused (and
		// woken), not abandoned alongside a new one.
		active, ensureErr := h.orchestrator.EnsureSandboxActive(ctx, sb)
		if ensureErr != nil {
			return nil, fmt.Errorf("ensure existing agent sandbox active: %w", ensureErr)
		}
		sb = active
		if h.orchestrator.NeedsURLRefresh(sb) {
			if err := h.orchestrator.RefreshAgentSandboxURL(ctx, sb); err != nil {
				return nil, err
			}
		}
		return sb, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load agent sandbox: %w", err)
	}
	secrets, prepErr := agentruntime.PrepareStartup(ctx, h.compileDeps, agent)
	if prepErr != nil {
		return nil, prepErr
	}
	created, err := h.orchestrator.CreateAgentSandbox(ctx, agent, secrets)
	if err != nil {
		return nil, err
	}
	if err := agentruntime.AttachProxyTokenToSandbox(ctx, h.compileDeps, agent, created.ID, secrets.ProxyTokenJTI); err != nil {
		return nil, fmt.Errorf("tag agent proxy token sandbox: %w", err)
	}
	return created, nil
}

// agentSandboxLockKey hashes an agent UUID to a bigint for pg_advisory_xact_lock.
// Collisions only serialise unrelated agents, which is harmless.
func agentSandboxLockKey(agentID uuid.UUID) int64 {
	return int64(binary.BigEndian.Uint64(agentID[:8])) // #nosec G115 -- hash truncation; sign bit is part of the hash distribution
}

func (h *AgentHandler) SyncOrgHivyAgent(ctx context.Context, orgID uuid.UUID) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		return fmt.Errorf("agent sandbox sync not configured")
	}
	agent, err := ensureHivyAgent(ctx, h.db, orgID)
	if err != nil {
		return fmt.Errorf("ensure Hivy agent: %w", err)
	}
	if h.memoryBanks != nil {
		if err := h.memoryBanks.EnsureOrgBank(ctx, orgID); err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("sync org Hivy agent: ensure memory bank: %w", err), map[string]any{
				"org_id":   orgID.String(),
				"agent_id": agent.ID.String(),
			})
		}
	}
	if _, _, err := h.SyncAgent(ctx, agent); err != nil {
		return err
	}
	return nil
}

func (h *AgentHandler) SyncAgent(ctx context.Context, agent *model.Agent) (*model.Sandbox, *agentruntime.SyncResponse, error) {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil {
		return nil, nil, fmt.Errorf("agent sandbox sync not configured")
	}
	sb, err := h.ensureAgentSandbox(ctx, agent)
	if err != nil {
		return nil, nil, fmt.Errorf("ensure agent sandbox: %w", err)
	}
	resp, err := h.runAgentSync(ctx, agent, sb)
	if err != nil {
		return nil, nil, fmt.Errorf("sync agent sandbox: %w", err)
	}
	return sb, resp, nil
}

func (h *AgentHandler) runAgentSync(ctx context.Context, agent *model.Agent, sb *model.Sandbox) (*agentruntime.SyncResponse, error) {
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt runtime secret: %w", err)
	}
	proxyToken, err := agentruntime.MintProxyToken(ctx, h.compileDeps, agent, sb.ID)
	if err != nil {
		return nil, fmt.Errorf("mint proxy token: %w", err)
	}
	runtimeEnv, err := agentruntime.BuildRuntimeEnvWithProxyToken(ctx, h.compileDeps, agent, sb, apiKey, proxyToken)
	if err != nil {
		return nil, fmt.Errorf("load runtime env: %w", err)
	}
	def, err := agentruntime.CompileWithProxyToken(ctx, h.compileDeps, agent, proxyToken)
	if err != nil {
		return nil, fmt.Errorf("compile: %w", err)
	}
	def.OutboundChannels = agentruntime.ControlPlaneOutboundChannels(h.compileDeps.Cfg, sb.ID)

	client, err := h.orchestrator.GetRuntimeClient(ctx, sb)
	if err != nil {
		return nil, fmt.Errorf("agent runtime client: %w", err)
	}

	resp, err := client.PutRuntimeConfig(ctx, agentruntime.ConfigUpdateRequest{
		Definition: def,
		RuntimeEnv: runtimeEnv,
	})
	if err != nil {
		return nil, err
	}

	if err := client.Readyz(ctx); err != nil {
		return nil, fmt.Errorf("agent runtime readyz: %w", err)
	}
	h.scheduleAgentProxyTokenRefresh(ctx, agent, sb)
	if agent.Status != "active" {
		if agent.OrgID == nil {
			return nil, fmt.Errorf("mark agent active: missing org_id")
		}
		if err := h.db.WithContext(ctx).Model(&model.Agent{}).
			Where("id = ? AND org_id = ?", agent.ID, *agent.OrgID).
			Update("status", "active").Error; err != nil {
			return nil, fmt.Errorf("mark agent active: %w", err)
		}
		agent.Status = "active"
	}

	return resp, nil
}

func (h *AgentHandler) scheduleExistingAgentProxyTokenRefresh(ctx context.Context, agent *model.Agent) {
	if h == nil || h.db == nil || agent == nil || agent.OrgID == nil {
		return
	}
	sb, err := h.mainAgentRuntimeSelector().MainRuntime(ctx, *agent.OrgID, agent.ID)
	if err != nil {
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			logging.Capture(ctx, fmt.Errorf("load agent sandbox for proxy token refresh schedule: %w", err))
		}
		return
	}
	h.scheduleAgentProxyTokenRefresh(ctx, agent, sb)
}

func (h *AgentHandler) scheduleAgentProxyTokenRefresh(ctx context.Context, agent *model.Agent, sb *model.Sandbox) {
	if err := tasks.ScheduleAgentProxyTokenRefresh(ctx, h.db, h.enqueuer, agent, sb); err != nil {
		logging.Capture(ctx, fmt.Errorf("schedule agent proxy token refresh: %w", err))
	}
}

func (h *AgentHandler) loadRuntimeEnv(ctx context.Context, agent *model.Agent, sb *model.Sandbox, runtimeSecret string) (map[string]string, error) {
	return agentruntime.BuildRuntimeEnv(ctx, h.compileDeps, agent, sb, runtimeSecret)
}

func (h *AgentHandler) mainAgentRuntimeSelector() agentsandbox.Selector {
	selector := agentsandbox.Selector{DB: h.db}
	if h != nil && h.compileDeps.Cfg != nil {
		selector.AgentRuntimeImage = h.compileDeps.Cfg.SandboxesRuntimeBaseImage
	}
	return selector
}

func toSyncResponseDTO(resp *agentruntime.SyncResponse) syncAgentResponse {
	out := syncAgentResponse{}
	if resp == nil {
		return out
	}
	out.Applied = resp.Applied
	out.Deleted = resp.Deleted
	out.ReposCloned = resp.ReposCloned
	out.RestartTriggered = resp.RestartTriggered
	out.Errors = resp.Errors
	return out
}
