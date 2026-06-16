package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const (
	agentProxyTokenRefreshLead    = 4 * time.Hour
	agentProxyTokenRefreshTimeout = 2 * time.Minute
	agentProxyTokenRefreshDedupe  = time.Hour
	// agentProxyTokenRevokeGrace protects tokens minted by a concurrent sync
	// from an in-flight refresh: revoking only tokens older than this window lets a
	// just-minted token survive until the runtime receives its config push.
	agentProxyTokenRevokeGrace = 10 * time.Minute
)

type AgentProxyTokenRefreshPayload struct {
	AgentID     uuid.UUID `json:"agent_id"`
	SandboxID   uuid.UUID `json:"sandbox_id"`
	ScheduledAt time.Time `json:"scheduled_at"`
}

func ScheduleAgentProxyTokenRefresh(ctx context.Context, db *gorm.DB, enqueuer enqueue.TaskEnqueuer, agent *model.Agent, sb *model.Sandbox) error {
	if db == nil || enqueuer == nil || agent == nil || agent.OrgID == nil || sb == nil || sb.ID == uuid.Nil {
		return nil
	}
	if agent.ID == uuid.Nil {
		return nil
	}
	if sb.AgentID == nil || *sb.AgentID != agent.ID {
		return nil
	}
	scheduledAt, err := nextAgentProxyTokenRefreshAt(ctx, db, agent, sb.ID, time.Now().UTC())
	if err != nil {
		return err
	}
	task, opts, err := NewAgentProxyTokenRefreshTask(AgentProxyTokenRefreshPayload{
		AgentID:     agent.ID,
		SandboxID:   sb.ID,
		ScheduledAt: scheduledAt,
	})
	if err != nil {
		return err
	}
	if _, err := enqueuer.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return fmt.Errorf("enqueue agent proxy token refresh: %w", err)
	}
	return nil
}

func NewAgentProxyTokenRefreshTask(payload AgentProxyTokenRefreshPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.AgentID == uuid.Nil || payload.SandboxID == uuid.Nil {
		return nil, nil, fmt.Errorf("agent proxy token refresh payload missing ids")
	}
	if payload.ScheduledAt.IsZero() {
		payload.ScheduledAt = time.Now().UTC()
	}
	payload.ScheduledAt = payload.ScheduledAt.UTC()
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal agent proxy token refresh payload: %w", err)
	}
	now := time.Now().UTC()
	uniqueTTL := time.Until(payload.ScheduledAt) + agentProxyTokenRefreshDedupe
	if uniqueTTL < agentProxyTokenRefreshDedupe {
		uniqueTTL = agentProxyTokenRefreshDedupe
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(6),
		asynq.Timeout(agentProxyTokenRefreshTimeout),
		asynq.Unique(uniqueTTL),
		asynq.TaskID(AgentProxyTokenRefreshTaskID(payload.SandboxID, payload.ScheduledAt)),
	}
	if payload.ScheduledAt.After(now) {
		opts = append(opts, asynq.ProcessAt(payload.ScheduledAt))
	}
	return asynq.NewTask(TypeAgentProxyTokenRefresh, body), opts, nil
}

func AgentProxyTokenRefreshTaskID(sandboxID uuid.UUID, scheduledAt time.Time) string {
	return fmt.Sprintf("agent-proxy-token-refresh:%s:%d", sandboxID, scheduledAt.UTC().Unix())
}

type AgentProxyTokenRefreshHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
}

func NewAgentProxyTokenRefreshHandler(
	db *gorm.DB,
	orchestrator *sandbox.Orchestrator,
	compileDeps agentruntime.CompileDeps,
	enqueuer enqueue.TaskEnqueuer,
) *AgentProxyTokenRefreshHandler {
	return &AgentProxyTokenRefreshHandler{
		db:           db,
		orchestrator: orchestrator,
		compileDeps:  compileDeps,
		enqueuer:     enqueuer,
	}
}

func (h *AgentProxyTokenRefreshHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.orchestrator == nil || h.compileDeps.EncKey == nil || h.enqueuer == nil {
		return fmt.Errorf("agent proxy token refresh handler not configured")
	}
	var payload AgentProxyTokenRefreshPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal agent proxy token refresh payload: %w", err)
	}
	if payload.AgentID == uuid.Nil || payload.SandboxID == uuid.Nil {
		return fmt.Errorf("agent proxy token refresh payload missing ids")
	}
	return h.run(ctx, payload)
}

func (h *AgentProxyTokenRefreshHandler) run(ctx context.Context, payload AgentProxyTokenRefreshPayload) error {
	agent, sb, ok, err := h.loadAgentAndSandbox(ctx, payload)
	if err != nil {
		return err
	}
	if !ok {
		return nil
	}

	if h.orchestrator.NeedsURLRefresh(sb) {
		if err := h.orchestrator.RefreshAgentSandboxURL(ctx, sb); err != nil {
			return fmt.Errorf("refresh agent sandbox url: %w", err)
		}
	}
	apiKey, err := h.compileDeps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return fmt.Errorf("decrypt agent runtime secret: %w", err)
	}
	client := agentruntime.NewClient(sb.RuntimeURL, apiKey)
	if err := client.Healthz(ctx); err != nil {
		return fmt.Errorf("agent runtime healthz: %w", err)
	}

	configUpdate, refreshed, err := agentruntime.BuildAgentRuntimeConfigUpdate(ctx, h.compileDeps, agent, sb, apiKey)
	if err != nil {
		if refreshed != nil {
			h.revokeToken(ctx, refreshed.JTI)
		}
		return fmt.Errorf("build agent runtime config: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, configUpdate); err != nil {
		h.revokeToken(ctx, refreshed.JTI)
		return err
	}
	if err := client.Readyz(ctx); err != nil {
		h.revokeToken(ctx, refreshed.JTI)
		return fmt.Errorf("agent runtime readyz: %w", err)
	}
	if err := h.revokeOlderTokens(ctx, agent, sb, refreshed.JTI); err != nil {
		return err
	}
	if err := h.markAgentRefreshed(ctx, agent); err != nil {
		return err
	}
	if err := ScheduleAgentProxyTokenRefresh(ctx, h.db, h.enqueuer, agent, sb); err != nil {
		return err
	}
	logging.FromContext(ctx).InfoContext(ctx, "agent proxy token refreshed",
		"agent_id", agent.ID, "sandbox_id", sb.ID, "jti", refreshed.JTI)
	return nil
}

func (h *AgentProxyTokenRefreshHandler) markAgentRefreshed(ctx context.Context, agent *model.Agent) error {
	if agent == nil || agent.OrgID == nil {
		return nil
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(&model.Agent{}).
		Where("id = ? AND org_id = ?", agent.ID, *agent.OrgID).
		Update("last_proxy_token_refreshed_at", now).Error; err != nil {
		return fmt.Errorf("mark agent proxy token refreshed: %w", err)
	}
	agent.LastProxyTokenRefreshedAt = &now
	return nil
}

func (h *AgentProxyTokenRefreshHandler) loadAgentAndSandbox(ctx context.Context, payload AgentProxyTokenRefreshPayload) (*model.Agent, *model.Sandbox, bool, error) {
	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND harness = ? AND status <> ?", payload.AgentID, "agent-sandbox", "archived").
		First(&agent).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, false, nil
		}
		return nil, nil, false, fmt.Errorf("load agent for proxy token refresh: %w", err)
	}
	if agent.OrgID == nil {
		return nil, nil, false, nil
	}
	sb, err := agentRuntimeSelector(h.db, h.compileDeps).MainRuntimeByID(ctx, *agent.OrgID, agent.ID, payload.SandboxID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, false, nil
		}
		return nil, nil, false, fmt.Errorf("load agent sandbox for proxy token refresh: %w", err)
	}
	return &agent, sb, true, nil
}

func (h *AgentProxyTokenRefreshHandler) revokeOlderTokens(ctx context.Context, agent *model.Agent, sb *model.Sandbox, keepJTI string) error {
	if agent == nil || agent.OrgID == nil || sb == nil || keepJTI == "" {
		return nil
	}
	now := time.Now().UTC()
	// Only revoke tokens older than the grace window (see agentProxyTokenRevokeGrace):
	// a concurrent sync may have just minted one the runtime now relies on.
	revokeBefore := now.Add(-agentProxyTokenRevokeGrace)
	if err := h.db.WithContext(ctx).Model(&model.Token{}).
		Where("org_id = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND jti != ? AND revoked_at IS NULL AND created_at < ?",
			*agent.OrgID,
			model.TokenMetaAgentID, agent.ID.String(),
			model.TokenMetaType, model.TokenTypeAgentProxy,
			model.TokenMetaHarness, model.TokenHarnessAgentSandbox,
			keepJTI, revokeBefore).
		Update("revoked_at", now).Error; err != nil {
		return fmt.Errorf("revoke older agent proxy tokens: %w", err)
	}
	return nil
}

func (h *AgentProxyTokenRefreshHandler) revokeToken(ctx context.Context, jti string) {
	if jti == "" {
		return
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(&model.Token{}).
		Where("jti = ? AND revoked_at IS NULL", jti).
		Update("revoked_at", now).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("agent proxy token refresh: revoke failed token: %w", err))
	}
}

func nextAgentProxyTokenRefreshAt(ctx context.Context, db *gorm.DB, agent *model.Agent, sandboxID uuid.UUID, now time.Time) (time.Time, error) {
	if agent == nil || agent.OrgID == nil {
		return now.UTC(), nil
	}
	var tok model.Token
	err := db.WithContext(ctx).
		Where("org_id = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND revoked_at IS NULL",
			*agent.OrgID,
			model.TokenMetaAgentID, agent.ID.String(),
			model.TokenMetaType, model.TokenTypeAgentProxy,
			model.TokenMetaHarness, model.TokenHarnessAgentSandbox).
		Where("COALESCE(meta->>?, '') IN (?, '')", model.TokenMetaSandboxID, sandboxID.String()).
		Order("created_at DESC").
		First(&tok).Error
	if err == nil {
		return tok.ExpiresAt.Add(-agentProxyTokenRefreshLead).UTC(), nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return now.UTC(), nil
	}
	return time.Time{}, fmt.Errorf("load latest agent proxy token: %w", err)
}
