package agentruntime

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/token"
)

const (
	DefaultAgentModel       = "deepseek-v4-flash"
	proxyTokenTTL           = 24 * time.Hour
	managedAgentName        = "Hivy"
	managedAgentDescription = "Hivy is the organization's managed AI agent."
)

type CompileDeps struct {
	DB         *gorm.DB
	Picker     credentials.Picker
	KMS        *crypto.KeyWrapper
	EncKey     *crypto.SymmetricKey
	SigningKey []byte
	Cfg        *config.Config
	Nango      *nango.Client
}

type StartupSecrets struct {
	ProxyToken    string
	ProxyTokenJTI string
	ProxyExpires  time.Time
}

type ProxyTokenResult struct {
	Token     string
	JTI       string
	ExpiresAt time.Time
}

type AgentDefinition struct {
	Agent            AgentMeta                   `json:"agent"`
	SystemPrompt     SystemPromptConfig          `json:"system_prompt"`
	Model            ModelConfig                 `json:"model"`
	Limits           map[string]any              `json:"limits,omitempty"`
	Context          map[string]any              `json:"context,omitempty"`
	Tools            []map[string]any            `json:"tools"`
	McpServers       []any                       `json:"mcp_servers"`
	McpToolFilter    *model.ToolFilter           `json:"mcp_tool_filter,omitempty"`
	AutoLoadSkills   []model.AutoLoadSkill       `json:"auto_load_skills,omitempty"`
	OutboundChannels []any                       `json:"outbound_channels"`
	SubAgents        map[string]*AgentDefinition `json:"sub_agents,omitempty"`
}

type AgentMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ModelConfig struct {
	Provider         string            `json:"provider"`
	BaseURL          string            `json:"base_url"`
	ModelID          string            `json:"model_id"`
	CanonicalModelID string            `json:"canonical_model_id,omitempty"`
	ProviderID       string            `json:"provider_id,omitempty"`
	UpstreamModelID  string            `json:"upstream_model_id,omitempty"`
	ModelProfile     string            `json:"model_profile,omitempty"`
	ProviderOptions  map[string]any    `json:"provider_options,omitempty"`
	Capabilities     map[string]any    `json:"capabilities,omitempty"`
	APIKeyEnv        string            `json:"api_key_env"`
	Temperature      *float64          `json:"temperature,omitempty"`
	MaxOutputTokens  *uint32           `json:"max_output_tokens,omitempty"`
	ReasoningEffort  *string           `json:"reasoning_effort,omitempty"`
	ExtraHeaders     map[string]string `json:"extra_headers"`
	Fallback         *ModelConfig      `json:"fallback,omitempty"`
}

func PrepareStartup(ctx context.Context, deps CompileDeps, agent *model.Agent) (*StartupSecrets, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent runtime startup: agent must have org_id")
	}
	proxyToken, err := mintProxyToken(ctx, deps, agent, uuid.Nil)
	if err != nil {
		return nil, err
	}
	return &StartupSecrets{
		ProxyToken:    proxyToken.Token,
		ProxyTokenJTI: proxyToken.JTI,
		ProxyExpires:  proxyToken.ExpiresAt,
	}, nil
}

func MintProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, sandboxID uuid.UUID) (*ProxyTokenResult, error) {
	return mintProxyToken(ctx, deps, agent, sandboxID)
}

func mintProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, sandboxID uuid.UUID) (*ProxyTokenResult, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent runtime proxy token: agent must have org_id")
	}
	if deps.DB == nil {
		return nil, fmt.Errorf("agent runtime proxy token: db is required")
	}
	if len(deps.SigningKey) == 0 {
		return nil, fmt.Errorf("agent runtime proxy token: signing key is required")
	}
	modelID := strings.TrimSpace(agent.Model)
	if modelID == "" {
		modelID = DefaultAgentModel
	}
	cred, err := credentials.ResolveForModel(ctx, deps.DB, nil, *agent.OrgID, modelID)
	if err != nil {
		return nil, fmt.Errorf("resolve credential: %w", err)
	}
	rawToken, jti, err := token.Mint(
		deps.SigningKey,
		agent.OrgID.String(),
		cred.ID.String(),
		proxyTokenTTL,
		token.MintOptions{IsSystem: cred.OrgID == nil},
	)
	if err != nil {
		return nil, fmt.Errorf("mint proxy token: %w", err)
	}
	now := time.Now().UTC()
	meta := model.JSON{
		model.TokenMetaAgentID: agent.ID.String(),
		model.TokenMetaType:    model.TokenTypeAgentProxy,
		model.TokenMetaHarness: model.TokenHarnessAgentSandbox,
	}
	if sandboxID != uuid.Nil {
		meta[model.TokenMetaSandboxID] = sandboxID.String()
	}
	expiresAt := now.Add(proxyTokenTTL)
	dbToken := model.Token{
		OrgID:        *agent.OrgID,
		CredentialID: cred.ID,
		JTI:          jti,
		ExpiresAt:    expiresAt,
		Scopes:       model.JSON{},
		Meta:         meta,
	}
	if err := deps.DB.WithContext(ctx).Create(&dbToken).Error; err != nil {
		return nil, fmt.Errorf("persist proxy token: %w", err)
	}
	return &ProxyTokenResult{
		Token:     "ptok_" + rawToken,
		JTI:       jti,
		ExpiresAt: expiresAt,
	}, nil
}

func AttachProxyTokenToSandbox(ctx context.Context, deps CompileDeps, agent *model.Agent, sandboxID uuid.UUID, jti string) error {
	return attachProxyTokenToSandbox(ctx, deps, agent, sandboxID, jti)
}

// attachProxyTokenToSandbox binds the minted token by JTI (not "latest by created_at", which could
// tag a stale/concurrent row and leave the refresh scheduler on the wrong one).
func attachProxyTokenToSandbox(ctx context.Context, deps CompileDeps, agent *model.Agent, sandboxID uuid.UUID, jti string) error {
	if agent == nil || agent.OrgID == nil {
		return fmt.Errorf("attach proxy token: agent must have org_id")
	}
	if sandboxID == uuid.Nil {
		return fmt.Errorf("attach proxy token: sandbox id is required")
	}
	if deps.DB == nil {
		return fmt.Errorf("attach proxy token: db is required")
	}
	if strings.TrimSpace(jti) == "" {
		return fmt.Errorf("attach proxy token: jti is required")
	}
	now := time.Now().UTC()
	var tok model.Token
	q := deps.DB.WithContext(ctx).
		Where("jti = ? AND org_id = ? AND meta->>? = ? AND meta->>? = ? AND meta->>? = ? AND revoked_at IS NULL AND expires_at > ?",
			jti,
			*agent.OrgID,
			model.TokenMetaAgentID, agent.ID.String(),
			model.TokenMetaType, model.TokenTypeAgentProxy,
			model.TokenMetaHarness, model.TokenHarnessAgentSandbox,
			now)
	if err := q.First(&tok).Error; err != nil {
		return fmt.Errorf("attach proxy token: load minted token %s: %w", jti, err)
	}
	meta := tok.Meta
	if meta == nil {
		meta = model.JSON{}
	}
	meta[model.TokenMetaSandboxID] = sandboxID.String()
	return deps.DB.WithContext(ctx).Model(&tok).Update("meta", meta).Error
}

func Compile(ctx context.Context, deps CompileDeps, agent *model.Agent) (*AgentDefinition, error) {
	return compile(ctx, deps, agent, nil)
}

func CompileWithProxyToken(ctx context.Context, deps CompileDeps, agent *model.Agent, proxyToken *ProxyTokenResult) (*AgentDefinition, error) {
	if proxyToken == nil || proxyToken.JTI == "" || proxyToken.Token == "" {
		return nil, fmt.Errorf("agent runtime compile: proxy token is required")
	}
	return compile(ctx, deps, agent, proxyToken)
}

func compile(ctx context.Context, deps CompileDeps, agent *model.Agent, proxyToken *ProxyTokenResult) (*AgentDefinition, error) {
	if agent == nil || agent.OrgID == nil {
		return nil, fmt.Errorf("agent runtime compile: agent must have org_id")
	}
	totalStarted := time.Now()
	phaseStarted := totalStarted
	logPhase := func(phase string, attrs ...any) {
		attrs = append(attrs,
			"total_ms", time.Since(totalStarted).Milliseconds(),
			"agent_id", agent.ID,
			"org_id", *agent.OrgID,
		)
		logging.LogPhase(ctx, "agent runtime compile phase", phase, phaseStarted, attrs...)
		phaseStarted = time.Now()
	}
	logPhase("start", "has_proxy_token", proxyToken != nil, "model", strings.TrimSpace(agent.Model))
	mcpServers := jsonArray(agent.McpServers)
	ourMCP := buildHivyMCPServer(ctx, deps, agent)
	if proxyToken != nil {
		ourMCP = buildAgentMCPServerWithToken(deps, proxyToken)
	}
	if ourMCP != nil {
		mcpServers = upsertHivyMCPServer(mcpServers, ourMCP)
	}
	logPhase("build mcp servers", "mcp_server_count", len(mcpServers), "has_hivy_mcp", ourMCP != nil)
	description := managedAgentDescription
	modelID := strings.TrimSpace(agent.Model)
	if modelID == "" {
		modelID = DefaultAgentModel
	}
	fragments := buildPromptSections(ctx, deps.DB, agent, description, modelID, deps.Cfg.PreviewBaseDomain)
	logPhase("build prompt sections", "model", modelID)
	mcpToolFilter := resolveAgentMCPToolFilter(ctx, deps.DB, agent)
	modelRoute := resolveAgentModelRouteMetadata(ctx, deps, agent, modelID)
	logPhase("resolve model route", "provider_id", modelRoute.ProviderID, "canonical_model_id", modelRoute.CanonicalModelID, "upstream_model_id", modelRoute.UpstreamModelID)
	tools, err := buildRuntimeTools(ctx, deps.DB, agent)
	if err != nil {
		return nil, err
	}
	logPhase("build tools", "tool_count", len(tools))
	subAgents, err := buildSubAgents(ctx, deps, agent, modelID)
	if err != nil {
		return nil, err
	}
	logPhase("build subagents", "subagent_count", len(subAgents))
	logPhase("complete",
		"tool_count", len(tools),
		"mcp_server_count", len(mcpServers),
		"subagent_count", len(subAgents),
	)
	return &AgentDefinition{
		Agent: AgentMeta{
			Name:        managedAgentName,
			Description: description,
		},
		SystemPrompt:     buildAgentSystemPrompt(ctx, fragments),
		Model:            proxyModel(deps.Cfg, modelID, modelRoute),
		Limits:           defaultLimits(),
		Tools:            tools,
		McpServers:       mcpServers,
		McpToolFilter:    mcpToolFilter,
		AutoLoadSkills:   compileAutoLoadSkills(ctx, deps.DB, agent, agent.AutoLoadSkills),
		OutboundChannels: []any{},
		SubAgents:        subAgents,
	}, nil
}

func ControlPlaneOutboundChannels(cfg *config.Config, sandboxID uuid.UUID) []any {
	return []any{
		map[string]any{
			"name":       "control-plane-runtime-stream",
			"type":       "websocket",
			"url":        RuntimeEventWebSocketURL(cfg, sandboxID),
			"secret_env": AgentEnvRuntimeSecret,
			"event_filter": []string{
				"user.message.received",
				"turn_started",
				"token",
				"thinking",
				"tool_call",
				"tool_result",
				"final",
				"turn_completed",
				"turn_failed",
				"question_requested",
				"question_answered",
				"plan_updated",
				"subagent_started",
				"subagent_completed",
				"subagent_errored",
				"model_usage",
				"error",
				"session_waiting",
			},
		},
		map[string]any{
			"name":       "control-plane-turn-state",
			"type":       "webhook",
			"url":        RuntimeTurnStateWebhookURL(cfg, sandboxID),
			"secret_env": AgentEnvRuntimeSecret,
			"event_filter": []string{
				"turn_started",
				"turn_completed",
				"turn_failed",
				"turn_interrupted",
			},
		},
	}
}
