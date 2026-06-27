package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func buildSubAgents(ctx context.Context, deps CompileDeps, agent *model.Agent, parentModelID string, parentSkills []SkillSpec) (map[string]*AgentDefinition, error) {
	specs, err := loadCatalogSubAgents(ctx, deps.DB, agent)
	if err != nil {
		return nil, err
	}
	out := make(map[string]*AgentDefinition, len(specs))
	keys := make([]string, 0, len(specs))
	for key := range specs {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		def, err := compileSubAgent(ctx, deps, agent, key, specs[key], parentModelID, parentSkills)
		if err != nil {
			return nil, err
		}
		out[key] = def
	}
	return out, nil
}

func loadCatalogSubAgents(ctx context.Context, db *gorm.DB, agent *model.Agent) (map[string]model.AgentCatalogSubAgent, error) {
	if agent == nil {
		return map[string]model.AgentCatalogSubAgent{}, nil
	}
	raw := agentCatalogSubAgentsRaw(agent)
	if len(raw) == 0 && db != nil && agent.AgentCatalogID != nil {
		var catalog model.AgentCatalog
		err := db.WithContext(ctx).
			Select("sub_agents").
			Where("id = ? AND status = ?", *agent.AgentCatalogID, model.AgentCatalogStatusActive).
			First(&catalog).Error
		if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, fmt.Errorf("load catalog subagents: %w", err)
		}
		raw = catalog.SubAgents
	}
	rawText := strings.TrimSpace(string(raw))
	if rawText == "" || rawText == "{}" {
		return map[string]model.AgentCatalogSubAgent{}, nil
	}
	var specs map[string]model.AgentCatalogSubAgent
	if err := json.Unmarshal([]byte(rawText), &specs); err != nil {
		return nil, fmt.Errorf("decode catalog subagents: %w", err)
	}
	return specs, nil
}

func agentCatalogSubAgentsRaw(agent *model.Agent) model.RawJSON {
	if agent == nil || agent.AgentCatalog == nil {
		return nil
	}
	return agent.AgentCatalog.SubAgents
}

func compileSubAgent(ctx context.Context, deps CompileDeps, parent *model.Agent, key string, spec model.AgentCatalogSubAgent, parentModelID string, parentSkills []SkillSpec) (*AgentDefinition, error) {
	name := strings.TrimSpace(spec.Name)
	if name == "" {
		name = key
	}
	description := strings.TrimSpace(spec.Description)
	if description == "" {
		description = "Investigates delegated work for the parent agent."
	}
	modelID := strings.TrimSpace(spec.Model)
	if modelID == "" {
		modelID = parentModelID
	}
	instructions := strings.TrimSpace(spec.Instructions)
	subAgent := &model.Agent{
		ID:           parent.ID,
		OrgID:        parent.OrgID,
		Name:         name,
		Description:  &description,
		Instructions: &instructions,
		Model:        modelID,
		Tools:        subAgentToolSelection(spec.Tools),
		McpServers:   model.RawJSON("[]"),
	}
	tools, err := buildRuntimeToolsFromSelection(subAgent.Tools)
	if err != nil {
		return nil, fmt.Errorf("compile subagent %q tools: %w", key, err)
	}
	modelRoute := resolveAgentModelRouteMetadata(ctx, deps, subAgent, modelID)
	return &AgentDefinition{
		Agent: AgentMeta{
			Name:        name,
			Description: description,
		},
		SystemPrompt:     buildAgentSystemPrompt(ctx, buildPromptSections(ctx, deps.DB, subAgent, description, modelID)),
		Model:            proxyModel(deps.Cfg, modelID, modelRoute),
		Limits:           defaultLimits(),
		Tools:            tools,
		McpServers:       []any{},
		McpToolFilter:    normalizeToolFilter(spec.McpToolFilter),
		Skills:           copySkillSpecs(parentSkills),
		SkillFilter:      normalizeSkillFilter(spec.SkillFilter),
		OutboundChannels: []any{},
		SubAgents:        map[string]*AgentDefinition{},
	}, nil
}

func subAgentToolSelection(selection model.JSON) model.JSON {
	out := make(model.JSON, len(selection)+2)
	for key, value := range selection {
		out[key] = value
	}
	out["skills_list"] = true
	out["skill_view"] = true
	return out
}
