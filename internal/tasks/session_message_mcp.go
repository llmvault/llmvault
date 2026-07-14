package tasks

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

// syncRuntimeMCPConfig reloads a session runtime when either the human actor or
// the org's monotonic MCP revision changes. Actor changes protect shared web
// sessions; revision changes make grant and credential revocation effective
// before the next turn without pushing secrets from an HTTP handler.
func (h *SessionMessageDeliverHandler) syncRuntimeMCPConfig(ctx context.Context, session *model.Session, agent *model.Agent, sb *model.Sandbox, turnActor *uuid.UUID) error {
	if h == nil || session == nil || agent == nil || sb == nil {
		return nil
	}
	mcpContext := agentruntime.MCPRuntimeContextForSession(*session, turnActor)
	desired := runtimeMCPActorID(mcpContext)
	configVersion, err := agentruntime.MCPConfigVersion(ctx, h.db, session.OrgID)
	if err != nil {
		return err
	}
	if equalRuntimeMCPActor(session.RuntimeMCPActorUserID, desired) && session.RuntimeMCPConfigVersion == configVersion {
		return nil
	}
	_, opts := sessionRuntimeAgent(agent, *session)
	opts.MCPContext = mcpContext
	if err := agentruntime.PushAgentRuntimeConfigWithProxyTokenOptions(ctx, h.compileDeps, agent, sb, nil, opts); err != nil {
		return fmt.Errorf("reload runtime MCP config: %w", err)
	}
	if err := h.db.WithContext(ctx).Model(&model.Session{}).
		Where("id = ? AND org_id = ?", session.ID, session.OrgID).
		Updates(map[string]any{
			"runtime_mcp_actor_user_id":  desired,
			"runtime_mcp_config_version": configVersion,
		}).Error; err != nil {
		return fmt.Errorf("persist runtime MCP config revision: %w", err)
	}
	session.RuntimeMCPActorUserID = desired
	session.RuntimeMCPConfigVersion = configVersion
	return nil
}

func equalRuntimeMCPActor(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
