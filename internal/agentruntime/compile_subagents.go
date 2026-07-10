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

func buildSubAgents(ctx context.Context, deps CompileDeps, agent *model.Agent, parentModelID string) (map[string]*AgentDefinition, error) {
	specs, err := loadSubAgentSpecs(ctx, deps.DB, agent)
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
		def, err := compileSubAgent(ctx, deps, agent, key, specs[key], parentModelID)
		if err != nil {
			return nil, err
		}
		out[key] = def
	}
	return out, nil
}

// loadSubAgentSpecs resolves an agent's sub-agent specs from the right source:
// catalog-linked agents keep their sub-agents in agent_catalog.sub_agents, while
// user-created agents (no catalog link) own sub-agents as rows in the agents table.
func loadSubAgentSpecs(ctx context.Context, db *gorm.DB, agent *model.Agent) (map[string]model.AgentCatalogSubAgent, error) {
	if agent == nil {
		return map[string]model.AgentCatalogSubAgent{}, nil
	}
	// Catalog-linked agents (identified by a catalog id or a preloaded catalog
	// association) keep their sub-agents in agent_catalog.sub_agents. Only
	// user-created agents with no catalog link own sub-agents as agents rows.
	if agent.AgentCatalogID != nil || agent.AgentCatalog != nil {
		return loadCatalogSubAgents(ctx, db, agent)
	}
	return loadDBSubAgents(ctx, db, agent)
}

// loadDBSubAgents loads a user-created agent's sub-agents from the agents table,
// mapping each row into the same spec shape the compiler already understands.
func loadDBSubAgents(ctx context.Context, db *gorm.DB, agent *model.Agent) (map[string]model.AgentCatalogSubAgent, error) {
	out := map[string]model.AgentCatalogSubAgent{}
	if db == nil || agent == nil {
		return out, nil
	}
	var rows []model.Agent
	if err := db.WithContext(ctx).
		Where("parent_agent_id = ? AND type = ? AND status = ?", agent.ID, model.AgentTypeSubAgent, "active").
		Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load db subagents: %w", err)
	}
	for _, row := range rows {
		out[row.ID.String()] = model.AgentCatalogSubAgent{
			Name:           row.Name,
			Description:    strings.TrimSpace(derefString(row.Description)),
			Model:          strings.TrimSpace(row.Model),
			Tools:          row.Tools,
			McpToolFilter:  row.McpToolFilter,
			AutoLoadSkills: append(model.AutoLoadSkills(nil), row.AutoLoadSkills...),
			Instructions:   strings.TrimSpace(derefString(row.Instructions)),
		}
	}
	return out, nil
}

func derefString(value *string) string {
	if value == nil {
		return ""
	}
	return *value
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

func compileSubAgent(ctx context.Context, deps CompileDeps, parent *model.Agent, key string, spec model.AgentCatalogSubAgent, parentModelID string) (*AgentDefinition, error) {
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
	tools, err := buildSubAgentRuntimeTools(subAgent.Tools)
	if err != nil {
		return nil, fmt.Errorf("compile subagent %q tools: %w", key, err)
	}
	modelRoute := resolveAgentModelRouteMetadata(ctx, deps, subAgent, modelID)
	return &AgentDefinition{
		Agent: AgentMeta{
			Name:        name,
			Description: description,
		},
		SystemPrompt:     buildAgentSystemPrompt(ctx, buildPromptSections(ctx, deps.DB, subAgent, description, modelID, deps.Cfg.PreviewBaseDomain)),
		Model:            proxyModel(deps.Cfg, modelID, modelRoute),
		Limits:           defaultLimits(),
		Tools:            tools,
		McpServers:       []any{},
		McpToolFilter:    normalizeToolFilter(spec.McpToolFilter),
		AutoLoadSkills:   compileAutoLoadSkills(ctx, deps.DB, parent, spec.AutoLoadSkills),
		OutboundChannels: []any{},
		SubAgents:        map[string]*AgentDefinition{},
	}, nil
}

// buildSubAgentRuntimeTools compiles a sub-agent's runtime tools, defaulting an
// EMPTY tool selection to read_file instead of zero tools. An empty selection
// previously compiled to a tool-less, useless sub-agent (the runtime
// AgentDefinition.tools has no omitempty). Sub-agents that need shell-based
// fd/rg discovery receive bash explicitly. This applies to both DB and catalog
// sub-agents by design.
func buildSubAgentRuntimeTools(selection model.JSON) ([]map[string]any, error) {
	if len(selection) == 0 {
		selection = defaultSubAgentToolSelection()
	}
	return buildRuntimeToolsFromSelection(selection)
}

func defaultSubAgentToolSelection() model.JSON {
	return model.JSON{
		"read_file": true,
	}
}

func subAgentToolSelection(selection model.JSON) model.JSON {
	out := make(model.JSON, len(selection))
	for key, value := range selection {
		out[key] = value
	}
	return out
}
