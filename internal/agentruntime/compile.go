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
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/token"
)

const (
	DefaultAgentModel           = "deepseek-v4-flash"
	DefaultAgentMultimodalModel = "gemini-3-flash-preview"
	proxyTokenTTL               = 24 * time.Hour
	managedAgentName            = "Hivy"
	managedAgentDescription     = "Hivy is the organization's managed AI agent."
)

type CompileDeps struct {
	DB         *gorm.DB
	Picker     credentials.Picker
	KMS        *crypto.KeyWrapper
	EncKey     *crypto.SymmetricKey
	SigningKey []byte
	Cfg        *config.Config
	Nango      *nango.Client
	Canvas     CanvasRuntimeEnvProvider
}

type CanvasRuntimeEnvProvider interface {
	AgentRuntimeEnv(ctx context.Context, agent *model.Agent) (map[string]string, error)
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
	MultimodalModel  *ModelConfig                `json:"multimodal_model,omitempty"`
	Limits           map[string]any              `json:"limits,omitempty"`
	Context          map[string]any              `json:"context,omitempty"`
	Tools            []map[string]any            `json:"tools,omitempty"`
	McpServers       []any                       `json:"mcp_servers"`
	Skills           []SkillSpec                 `json:"skills"`
	OutboundChannels []any                       `json:"outbound_channels"`
	SubAgents        map[string]*AgentDefinition `json:"sub_agents,omitempty"`
}

type AgentMeta struct {
	Name        string `json:"name"`
	Description string `json:"description"`
}

type ModelConfig struct {
	Provider        string            `json:"provider"`
	BaseURL         string            `json:"base_url"`
	ModelID         string            `json:"model_id"`
	APIKeyEnv       string            `json:"api_key_env"`
	Temperature     *float64          `json:"temperature,omitempty"`
	MaxOutputTokens *uint32           `json:"max_output_tokens,omitempty"`
	ReasoningEffort *string           `json:"reasoning_effort,omitempty"`
	ExtraHeaders    map[string]string `json:"extra_headers"`
	Fallback        *ModelConfig      `json:"fallback,omitempty"`
}

type SkillSpec struct {
	Name                         string            `json:"name"`
	Description                  string            `json:"description"`
	Trigger                      map[string]any    `json:"trigger"`
	Instructions                 string            `json:"instructions"`
	Files                        map[string]string `json:"files,omitempty"`
	Category                     *string           `json:"category,omitempty"`
	Tags                         []string          `json:"tags"`
	RelatedSkills                []string          `json:"related_skills"`
	RequiredEnvironmentVariables []string          `json:"required_environment_variables"`
	RequiredCredentialFiles      []string          `json:"required_credential_files"`
	Pinned                       bool              `json:"pinned"`
}

type MemoryContext struct {
	Entries     []MemoryContextEntry `json:"entries"`
	TokenBudget int                  `json:"token_budget"`
}

type MemoryContextEntry struct {
	Content    string   `json:"content"`
	Source     string   `json:"source,omitempty"`
	MemoryType string   `json:"memory_type,omitempty"`
	Tags       []string `json:"tags,omitempty"`
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
	skills, err := buildSkills(ctx, deps.DB, agent.ID)
	if err != nil {
		return nil, err
	}
	mcpServers := jsonArray(agent.McpServers)
	ourMCP := buildHivyMCPServer(ctx, deps, agent)
	if proxyToken != nil {
		ourMCP = buildAgentMCPServerWithToken(deps, proxyToken)
	}
	if ourMCP != nil {
		mcpServers = upsertHivyMCPServer(mcpServers, ourMCP)
	}
	description := managedAgentDescription
	fragments := buildPromptSections(ctx, deps.DB, agent, description)
	contextMap := map[string]any{
		"memory": emptyMemoryContext(),
	}
	modelID := strings.TrimSpace(agent.Model)
	if modelID == "" {
		modelID = DefaultAgentModel
	}
	tools, err := buildRuntimeTools(ctx, deps.DB, agent)
	if err != nil {
		return nil, err
	}
	subAgents, err := buildSubAgents(ctx, deps, agent, modelID)
	if err != nil {
		return nil, err
	}
	return &AgentDefinition{
		Agent: AgentMeta{
			Name:        managedAgentName,
			Description: description,
		},
		SystemPrompt:     buildAgentSystemPrompt(ctx, fragments),
		Model:            proxyModel(deps.Cfg, modelID),
		MultimodalModel:  ptrModel(proxyModel(deps.Cfg, DefaultAgentMultimodalModel)),
		Limits:           defaultLimits(),
		Context:          contextMap,
		Tools:            tools,
		McpServers:       mcpServers,
		Skills:           skills,
		OutboundChannels: []any{},
		SubAgents:        subAgents,
	}, nil
}

func ControlPlaneOutboundChannels(cfg *config.Config, sandboxID uuid.UUID) []any {
	baseURL := cfg.RuntimeControlPlaneBaseURL()
	return []any{
		map[string]any{
			"name":       "control-plane-memory",
			"type":       "webhook",
			"url":        fmt.Sprintf("%s/internal/webhooks/agent/%s", baseURL, sandboxID),
			"secret_env": AgentEnvRuntimeSecret,
		},
	}
}
