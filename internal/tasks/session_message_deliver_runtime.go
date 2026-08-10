package tasks

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/observability/correlation"
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

func (h *SessionMessageDeliverHandler) ensureRuntimeClient(ctx context.Context, session model.Session, agent *model.Agent) (*model.Sandbox, *agentruntime.Client, sandbox.WakeReservation, error) {
	return h.ensureRuntimeClientUnlocked(ctx, session, agent)
}

func (h *SessionMessageDeliverHandler) ensureRuntimeClientUnlocked(ctx context.Context, session model.Session, agent *model.Agent) (*model.Sandbox, *agentruntime.Client, sandbox.WakeReservation, error) {
	reservation := sandbox.WakeReservation{}
	if h.compileDeps.EncKey == nil {
		return nil, nil, reservation, fmt.Errorf("session message delivery: runtime encryption key is required")
	}
	if agent == nil || agent.OrgID == nil {
		return nil, nil, reservation, fmt.Errorf("session message delivery: agent must have org_id")
	}
	draining, err := sessionRuntimeDraining(ctx, h.db, session)
	if err != nil {
		return nil, nil, reservation, fmt.Errorf("check session runtime drain status: %w", err)
	}
	if draining {
		return nil, nil, reservation, ErrSessionRuntimeDraining
	}
	sb, err := h.loadRuntimeSandbox(ctx, session, agent)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if !h.allowProvisioning {
			return nil, nil, reservation, ErrSessionRuntimeNotReady
		}
		provisioning := correlation.NewProvisioning(session.ID)
		provisioning.OrgID = session.OrgID.String()
		provisioning.AgentID = agent.ID.String()
		ctx = correlation.WithValues(ctx, provisioning)
		totalStarted := time.Now()
		phaseStarted := totalStarted
		logPhase := func(phase string, attrs ...any) {
			attrs = append(attrs, "total_ms", time.Since(totalStarted).Milliseconds())
			logging.LogPhase(ctx, "session lazy provision phase", phase, phaseStarted, attrs...)
			phaseStarted = time.Now()
		}
		logPhase("start")
		err = func() error {
			runtimeAgent, runtimeOptions := sessionRuntimeAgent(agent, session)
			runtimeOptions.TeamID = session.TeamID
			runtimeOptions.SessionID = session.ID
			runtimeOptions.ProvisioningAttemptID, _ = uuid.Parse(provisioning.ProvisioningAttemptID)
			runtimeOptions.TraceID = provisioning.TraceID
			mcpConfigVersion, versionErr := agentruntime.MCPConfigVersion(ctx, h.db, session.OrgID)
			if versionErr != nil {
				return versionErr
			}
			logPhase("load runtime config")
			secrets, prepErr := agentruntime.PrepareStartup(ctx, h.compileDeps, runtimeAgent)
			if prepErr != nil {
				return fmt.Errorf("prepare agent runtime startup: %w", prepErr)
			}
			logPhase("prepare startup")
			var createErr error
			sb, createErr = h.orchestrator.CreateAgentSandboxWithRuntimeOptions(ctx, runtimeAgent, secrets, runtimeOptions)
			if createErr != nil {
				return fmt.Errorf("create agent sandbox: %w", createErr)
			}
			ctx = correlation.WithSandboxID(ctx, sb.ID.String())
			logPhase("create agent sandbox", "sandbox_id", sb.ID)
			if err := agentruntime.AttachProxyTokenToSandbox(ctx, h.compileDeps, runtimeAgent, sb.ID, secrets.ProxyTokenJTI); err != nil {
				return fmt.Errorf("tag agent proxy token sandbox: %w", err)
			}
			logPhase("attach startup token", "sandbox_id", sb.ID)
			if err := h.db.WithContext(ctx).Model(&model.Session{}).
				Where("id = ? AND org_id = ?", session.ID, session.OrgID).
				Updates(map[string]any{
					"sandbox_id":                      sb.ID,
					"sandbox_vcpu":                    sb.VCPU,
					"sandbox_pricing_version":         billing.SandboxPricingVersion,
					"sandbox_credits_per_vcpu_minute": billing.SandboxCreditsPerVCPUMinute,
					"runtime_mcp_actor_user_id":       runtimeMCPActorID(runtimeOptions.MCPContext),
					"runtime_mcp_config_version":      mcpConfigVersion,
				}).Error; err != nil {
				return fmt.Errorf("attach session sandbox: %w", err)
			}
			logPhase("attach session sandbox", "sandbox_id", sb.ID)
			return nil
		}()
		if err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "session lazy provisioning failed",
				"event", "session provisioning",
				"phase", "provision sandbox",
				"status", "error",
				"duration_ms", time.Since(totalStarted).Milliseconds(),
				"provisioning_path", "lazy_message_delivery",
				"error", err,
			)
			return nil, nil, reservation, err
		}
		logPhase("complete", "sandbox_id", sb.ID)
		logging.FromContext(ctx).InfoContext(ctx, "session provisioning complete",
			"event", "session provisioning",
			"phase", "complete",
			"status", "success",
			"duration_ms", time.Since(totalStarted).Milliseconds(),
			"provisioning_path", "lazy_message_delivery",
			"sandbox_id", sb.ID,
		)
	} else if err != nil {
		return nil, nil, reservation, fmt.Errorf("load agent sandbox: %w", err)
	} else {
		reservation, err = sandbox.ReserveWake(ctx, h.db, session.OrgID, sb.ID)
		if err != nil {
			return nil, nil, reservation, err
		}
	}
	client, err := h.orchestrator.GetRuntimeClient(ctx, sb)
	if err != nil {
		h.rollbackWakeReservation(ctx, reservation)
		return nil, nil, reservation, fmt.Errorf("get runtime client: %w", err)
	}
	return sb, client, reservation, nil
}

func (h *SessionMessageDeliverHandler) commitWakeReservation(ctx context.Context, reservation sandbox.WakeReservation) {
	if err := reservation.Commit(ctx, h.db); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "commit session delivery wake reservation", "org_id", reservation.OrgID, "sandbox_id", reservation.SandboxID, "error", err)
	}
}

func (h *SessionMessageDeliverHandler) rollbackWakeReservation(ctx context.Context, reservation sandbox.WakeReservation) {
	cleanupCtx := context.WithoutCancel(ctx)
	if err := reservation.Rollback(cleanupCtx, h.db); err != nil {
		logging.FromContext(cleanupCtx).ErrorContext(cleanupCtx, "rollback session delivery wake reservation", "org_id", reservation.OrgID, "sandbox_id", reservation.SandboxID, "error", err)
	}
}

func runtimeMCPActorID(mcpContext agentruntime.MCPRuntimeContext) *uuid.UUID {
	if !mcpContext.AllowsPersonalServers() {
		return nil
	}
	return mcpContext.ActorUserID
}

func sessionRuntimeAgent(agent *model.Agent, session model.Session) (*model.Agent, agentruntime.RuntimeConfigOptions) {
	opts := agentruntime.RuntimeConfigOptions{MCPContext: agentruntime.MCPRuntimeContextForSession(session, nil)}
	if agent == nil {
		return agent, opts
	}
	runtimeAgent := *agent
	if modelID := strings.TrimSpace(session.Model); modelID != "" {
		runtimeAgent.Model = modelID
		opts.ModelID = modelID
	}
	opts.ReasoningEffort = strings.TrimSpace(session.ReasoningEffort)
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
