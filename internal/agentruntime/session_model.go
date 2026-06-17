package agentruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func SessionProxyAPIKeyEnv(sessionID uuid.UUID) string {
	if sessionID == uuid.Nil {
		return ProxyAPIKeyEnv
	}
	return "HIVY_SESSION_" + strings.ReplaceAll(sessionID.String(), "-", "_") + "_PROXY_API_KEY"
}

func PushAgentRuntimeConfigForSessionModel(
	ctx context.Context,
	deps CompileDeps,
	agent *model.Agent,
	sb *model.Sandbox,
	sessionID uuid.UUID,
	modelID string,
	reasoningEffort string,
) (*ModelConfig, error) {
	if deps.EncKey == nil {
		return nil, fmt.Errorf("agent runtime config push: encryption key is required")
	}
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent runtime config push: agent must have org_id")
	}
	if sb == nil {
		return nil, fmt.Errorf("agent runtime config push: sandbox is required")
	}
	runtimeSecret, err := deps.EncKey.DecryptString(sb.EncryptedRuntimeSecret)
	if err != nil {
		return nil, fmt.Errorf("decrypt runtime secret: %w", err)
	}
	configUpdate, _, err := BuildAgentRuntimeConfigUpdate(ctx, deps, agent, sb, runtimeSecret)
	if err != nil {
		return nil, fmt.Errorf("build agent runtime config: %w", err)
	}
	sessionModel, err := addSessionModelProxyToken(ctx, deps, agent, sb, sessionID, modelID, reasoningEffort, configUpdate.RuntimeEnv)
	if err != nil {
		return nil, err
	}
	client := NewClient(sb.RuntimeURL, runtimeSecret)
	if err := client.Healthz(ctx); err != nil {
		return nil, fmt.Errorf("agent runtime healthz: %w", err)
	}
	if _, err := client.PutRuntimeConfig(ctx, configUpdate); err != nil {
		return nil, fmt.Errorf("agent runtime put config: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		return nil, fmt.Errorf("agent runtime readyz: %w", err)
	}
	if err := RepointAgentSchedules(ctx, deps.DB, agent, sb); err != nil {
		return nil, fmt.Errorf("repoint agent schedules: %w", err)
	}
	return &sessionModel, nil
}

func addSessionModelProxyToken(
	ctx context.Context,
	deps CompileDeps,
	agent *model.Agent,
	sb *model.Sandbox,
	sessionID uuid.UUID,
	modelID string,
	reasoningEffort string,
	env map[string]string,
) (ModelConfig, error) {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		modelID = strings.TrimSpace(agent.Model)
	}
	if modelID == "" {
		modelID = DefaultAgentModel
	}
	sessionModel := ProxyModelConfig(deps.Cfg, modelID, reasoningEffort)
	baseModel := strings.TrimSpace(agent.Model)
	if baseModel == "" {
		baseModel = DefaultAgentModel
	}
	if modelID == baseModel {
		return sessionModel, nil
	}
	tokenAgent := *agent
	tokenAgent.Model = modelID
	sandboxID := uuid.Nil
	if sb != nil {
		sandboxID = sb.ID
	}
	token, err := MintProxyToken(ctx, deps, &tokenAgent, sandboxID)
	if err != nil {
		return ModelConfig{}, err
	}
	envKey := SessionProxyAPIKeyEnv(sessionID)
	if envKey == ProxyAPIKeyEnv {
		return ModelConfig{}, fmt.Errorf("session proxy api key env requires a session id")
	}
	env[envKey] = token.Token
	sessionModel.APIKeyEnv = envKey
	return sessionModel, nil
}
