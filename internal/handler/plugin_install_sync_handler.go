package handler

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"golang.org/x/sync/errgroup"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/tasks"
)

const (
	systemChannelName             = "system"
	pluginServiceDiscoverySource  = "plugin.service_discovery"
	pluginServiceDiscoveryJobKind = "plugin_service_discovery"
)

type PluginInstallSyncHandler struct {
	db             *gorm.DB
	agentHandler   *AgentHandler
	sessionHandler *SessionHandler
	enq            enqueue.TaskEnqueuer
}

func NewPluginInstallSyncHandler(
	db *gorm.DB,
	agentHandler *AgentHandler,
	enq enqueue.TaskEnqueuer,
	orchestrator *sandbox.Orchestrator,
	compileDeps agentruntime.CompileDeps,
) *PluginInstallSyncHandler {
	return &PluginInstallSyncHandler{
		db:             db,
		agentHandler:   agentHandler,
		sessionHandler: NewSessionHandler(db, enq).WithRuntimeDelivery(orchestrator, compileDeps),
		enq:            enq,
	}
}

func (h *PluginInstallSyncHandler) Handle(ctx context.Context, task *asynq.Task) error {
	if h == nil || h.db == nil || h.agentHandler == nil || h.sessionHandler == nil {
		return fmt.Errorf("plugin install sync handler not configured")
	}
	var payload tasks.PluginInstallSyncPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal plugin install sync payload: %w", err)
	}
	if payload.OrgID == uuid.Nil || payload.PluginID == uuid.Nil || payload.InstallID == uuid.Nil {
		return fmt.Errorf("org_id, plugin_id, and install_id are required")
	}
	var install model.OrgPluginInstall
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND plugin_id = ? AND revoked_at IS NULL", payload.InstallID, payload.OrgID, payload.PluginID).
		First(&install).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load plugin install: %w", err)
	}
	var plugin model.Plugin
	if err := h.db.WithContext(ctx).Where("id = ? AND status = ?", payload.PluginID, model.PluginStatusActive).First(&plugin).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load plugin: %w", err)
	}

	discoveryAgent, err := ensureHivyAgent(ctx, h.db, payload.OrgID)
	if err != nil {
		return fmt.Errorf("ensure Hivy agent: %w", err)
	}
	if err := h.syncAlwaysOnAgents(ctx, payload.OrgID); err != nil {
		return err
	}
	h.enqueuePluginResourceReconcile(ctx, payload.OrgID, plugin.ID)
	return h.dispatchPluginServiceDiscovery(ctx, payload, plugin, discoveryAgent)
}

func (h *PluginInstallSyncHandler) enqueuePluginResourceReconcile(ctx context.Context, orgID, pluginID uuid.UUID) {
	if h == nil || h.db == nil || h.enq == nil {
		return
	}
	var conns []model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Joins("JOIN plugin_integrations ON plugin_integrations.provider = integrations.provider AND plugin_integrations.kind = ? AND plugin_integrations.plugin_id = ?", model.PluginIntegrationKindIntegration, pluginID).
		Where("connections.org_id = ? AND connections.revoked_at IS NULL", orgID).
		Order("connections.created_at ASC").
		Find(&conns).Error; err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("list plugin resource connections: %w", err), map[string]any{
			"org_id":    orgID.String(),
			"plugin_id": pluginID.String(),
		})
		return
	}
	for _, conn := range conns {
		enqueueGitHubRepositoryCloneForAlwaysOnAgents(ctx, h.db, h.enq, orgID, conn)
	}
}

func (h *PluginInstallSyncHandler) syncAlwaysOnAgents(ctx context.Context, orgID uuid.UUID) error {
	var agents []model.Agent
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND sandbox_strategy = ? AND status <> ?", orgID, agentStrategyAlwaysOn, "archived").
		Order("created_at ASC").
		Find(&agents).Error; err != nil {
		return fmt.Errorf("load always-on agents: %w", err)
	}
	g, ctx := errgroup.WithContext(ctx)
	g.SetLimit(8)
	for i := range agents {
		agent := agents[i]
		g.Go(func() error {
			if _, _, err := h.agentHandler.SyncAgent(ctx, &agent); err != nil {
				return fmt.Errorf("sync agent %s: %w", agent.ID, err)
			}
			return nil
		})
	}
	if err := g.Wait(); err != nil {
		return fmt.Errorf("sync always-on agents: %w", err)
	}
	return nil
}

func (h *PluginInstallSyncHandler) dispatchPluginServiceDiscovery(
	ctx context.Context,
	payload tasks.PluginInstallSyncPayload,
	plugin model.Plugin,
	agent *model.Agent,
) error {
	if agent == nil || agent.ID == uuid.Nil {
		return fmt.Errorf("discovery agent is required")
	}
	if h.agentHandler.memoryBanks != nil {
		if err := h.agentHandler.memoryBanks.EnsureOrgBank(ctx, payload.OrgID); err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("plugin service discovery: ensure memory bank: %w", err), map[string]any{
				"org_id":    payload.OrgID.String(),
				"agent_id":  agent.ID.String(),
				"plugin_id": plugin.ID.String(),
			})
		}
	}
	channel, err := h.ensureSystemChannel(ctx, payload.OrgID, agent.ID)
	if err != nil {
		return err
	}
	connections, err := h.pluginServiceDiscoveryConnections(ctx, payload.OrgID, plugin.ID)
	if err != nil {
		return err
	}
	for _, conn := range connections {
		provider := strings.ToLower(strings.TrimSpace(conn.Integration.Provider))
		prompt := serviceDiscoveryPrompt(provider, conn)
		if prompt == "" {
			continue
		}
		_, _, created, err := h.sessionHandler.createSystemSession(ctx, systemSessionRequest{
			OrgID:             payload.OrgID,
			ChannelID:         channel.ID,
			Agent:             *agent,
			Text:              prompt,
			Source:            pluginServiceDiscoverySource,
			SourceID:          &payload.InstallID,
			SourceResourceKey: pluginServiceDiscoveryResourceKey(payload.InstallID, conn.ID),
			Raw: model.JSON{
				"kind":          pluginServiceDiscoveryJobKind,
				"plugin_id":     plugin.ID.String(),
				"plugin_slug":   plugin.Slug,
				"provider":      provider,
				"connection_id": conn.ID.String(),
			},
		})
		if err != nil {
			return fmt.Errorf("dispatch %s service discovery: %w", provider, err)
		}
		if created {
			logging.FromContext(ctx).InfoContext(ctx, "plugin service discovery dispatched",
				"org_id", payload.OrgID,
				"plugin_id", plugin.ID,
				"provider", provider,
				"connection_id", conn.ID)
		}
	}
	return nil
}

func (h *PluginInstallSyncHandler) ensureSystemChannel(ctx context.Context, orgID, agentID uuid.UUID) (model.Channel, error) {
	var channel model.Channel
	scope := model.Channel{OrgID: orgID, Origin: "native", Name: systemChannelName}
	attrs := model.Channel{
		Description:      "System-managed jobs",
		Kind:             "system",
		Visibility:       "private",
		DefaultAgentID:   agentID,
		ExternalMetadata: model.JSON{"source": "system"},
	}
	if err := h.db.WithContext(ctx).Where(&scope).Attrs(attrs).FirstOrCreate(&channel).Error; err != nil {
		return model.Channel{}, fmt.Errorf("ensure system channel: %w", err)
	}
	return channel, nil
}

func (h *PluginInstallSyncHandler) pluginServiceDiscoveryConnections(ctx context.Context, orgID, pluginID uuid.UUID) ([]model.Connection, error) {
	var reqs []model.PluginIntegration
	if err := h.db.WithContext(ctx).
		Where("plugin_id = ? AND kind = ?", pluginID, model.PluginIntegrationKindIntegration).
		Find(&reqs).Error; err != nil {
		return nil, fmt.Errorf("load plugin integrations: %w", err)
	}
	providers := make([]string, 0, len(reqs))
	seen := map[string]bool{}
	for _, req := range reqs {
		provider := strings.ToLower(strings.TrimSpace(req.Provider))
		if provider == "" || seen[provider] || !serviceDiscoveryProviderSupported(provider) {
			continue
		}
		seen[provider] = true
		providers = append(providers, provider)
	}
	if len(providers) == 0 {
		return nil, nil
	}
	var connections []model.Connection
	if err := h.db.WithContext(ctx).
		Preload("Integration").
		Joins("JOIN integrations ON integrations.id = connections.integration_id AND integrations.deleted_at IS NULL").
		Where("connections.org_id = ? AND connections.revoked_at IS NULL AND lower(integrations.provider) IN ?", orgID, providers).
		Order("connections.created_at ASC").
		Find(&connections).Error; err != nil {
		return nil, fmt.Errorf("load plugin service discovery connections: %w", err)
	}
	return connections, nil
}

func pluginServiceDiscoveryResourceKey(installID, connectionID uuid.UUID) string {
	return fmt.Sprintf("plugin-install:%s:service-discovery:%s", installID.String(), connectionID.String())
}
