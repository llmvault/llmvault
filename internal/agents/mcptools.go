package agents

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// Tool names. list_org_plugins, list_agents, and get_agent are read-only;
// create_agent and update_agent are privileged, opt-in mutating tools gated per
// calling agent.
const (
	toolListOrgPlugins = "list_org_plugins"
	toolListAgents     = "list_agents"
	toolGetAgent       = "get_agent"
	toolCreateAgent    = "create_agent"
	toolUpdateAgent    = "update_agent"
)

// NewToolsFunc returns the agent-builder ToolsFunc. It registers the read-only
// list_org_plugins / list_agents / get_agent tools and the mutating
// create_agent / update_agent tools on the MCP server ONLY when the calling
// agent is permitted (see agentBuilderEnabled): the org's default Hivy agent by
// default, or any agent whose McpToolFilter.Allow explicitly contains
// create_agent/update_agent. frontendURL is used to build the agent URL in tool
// responses.
func NewToolsFunc(deps Deps, frontendURL string) func(server *mcp.Server, token *model.Token) {
	return func(server *mcp.Server, token *model.Token) {
		if server == nil || deps.DB == nil || !agentProxyToken(token) {
			return
		}
		agentID, err := tokenAgentID(token)
		if err != nil {
			return
		}
		agent, err := loadOrgAgent(context.Background(), deps.DB, token.OrgID, agentID)
		if err != nil {
			return
		}
		if !agentBuilderEnabled(agent) {
			return
		}
		registerListOrgPlugins(server, deps.DB, token, frontendURL)
		registerListAgents(server, deps.DB, token)
		registerGetAgent(server, deps.DB, token, frontendURL)
		registerCreateAgent(server, deps, token, frontendURL)
		registerUpdateAgent(server, deps, token, frontendURL)
	}
}

// agentBuilderEnabled reports whether the calling agent may use the privileged
// agent-builder tools. The default Hivy agent gets them automatically. Any
// other agent must explicitly allow-list them in McpToolFilter.Allow. This is
// an intentional exception to the normal "nil filter = all MCP tools allowed"
// rule: these tools are never granted implicitly.
func agentBuilderEnabled(agent *model.Agent) bool {
	if agent == nil {
		return false
	}
	if agent.IsDefault {
		return true
	}
	if agent.McpToolFilter == nil {
		return false
	}
	for _, allowed := range agent.McpToolFilter.Allow {
		switch strings.TrimSpace(allowed) {
		case toolCreateAgent, toolUpdateAgent:
			return true
		}
	}
	return false
}

// --- list_org_plugins --------------------------------------------------------

func registerListOrgPlugins(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolListOrgPlugins,
		Description: "List the plugins available to this organization, split into installed and available. Each plugin lists its skills, required connections, and an install_url (the page to send the user to install/connect it); available plugins also list missing_requirements. Use this to discover which plugin_slugs and skills you can pass to create_agent / update_agent.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListOrgPlugins(ctx, db, token, frontendURL)
	})
}

func handleListOrgPlugins(ctx context.Context, db *gorm.DB, token *model.Token, frontendURL string) (*mcp.CallToolResult, error) {
	var plugins []model.Plugin
	if err := db.WithContext(ctx).Where("status = ?", model.PluginStatusActive).Order("name ASC").Find(&plugins).Error; err != nil {
		return toolError("failed to list plugins: " + err.Error()), nil
	}
	installedIDs, err := installedPluginIDSet(ctx, db, token.OrgID)
	if err != nil {
		return toolError(err.Error()), nil
	}
	installed := make([]map[string]any, 0)
	available := make([]map[string]any, 0)
	for _, plugin := range plugins {
		obj, err := pluginObject(ctx, db, token.OrgID, plugin, !installedIDs[plugin.ID], frontendURL)
		if err != nil {
			return toolError(err.Error()), nil
		}
		if installedIDs[plugin.ID] {
			installed = append(installed, obj)
		} else {
			available = append(available, obj)
		}
	}
	return toolJSON(map[string]any{
		"installed": installed,
		"available": available,
	})
}

func installedPluginIDSet(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (map[uuid.UUID]bool, error) {
	var pluginIDs []uuid.UUID
	if err := db.WithContext(ctx).Model(&model.OrgPluginInstall{}).
		Where("org_id = ? AND revoked_at IS NULL", orgID).
		Distinct("plugin_id").
		Pluck("plugin_id", &pluginIDs).Error; err != nil {
		return nil, fmt.Errorf("load org plugin installs: %w", err)
	}
	out := make(map[uuid.UUID]bool, len(pluginIDs))
	for _, id := range pluginIDs {
		out[id] = true
	}
	return out, nil
}

func pluginObject(ctx context.Context, db *gorm.DB, orgID uuid.UUID, plugin model.Plugin, includeMissing bool, frontendURL string) (map[string]any, error) {
	var skills []model.Skill
	if err := db.WithContext(ctx).
		Where("plugin_id = ? AND status = ?", plugin.ID, model.SkillStatusPublished).
		Order("name ASC").
		Find(&skills).Error; err != nil {
		return nil, fmt.Errorf("load plugin skills: %w", err)
	}
	skillObjs := make([]map[string]any, 0, len(skills))
	for _, skill := range skills {
		desc := ""
		if skill.Description != nil {
			desc = *skill.Description
		}
		skillObjs = append(skillObjs, map[string]any{
			"slug":        skill.Slug,
			"name":        skill.Name,
			"description": desc,
		})
	}
	var reqs []model.PluginIntegration
	if err := db.WithContext(ctx).Where("plugin_id = ?", plugin.ID).Order("provider ASC").Find(&reqs).Error; err != nil {
		return nil, fmt.Errorf("load plugin requirements: %w", err)
	}
	reqObjs := make([]map[string]any, 0, len(reqs))
	for _, req := range reqs {
		reqObjs = append(reqObjs, map[string]any{
			"provider": req.Provider,
			"kind":     req.Kind,
			"required": req.Required,
		})
	}
	obj := map[string]any{
		"id":                   plugin.ID.String(),
		"slug":                 plugin.Slug,
		"name":                 plugin.Name,
		"description":          plugin.Description,
		"category":             plugin.Category,
		"skills":               skillObjs,
		"required_connections": reqObjs,
		"install_url":          pluginInstallURL(frontendURL, plugin.Slug),
	}
	if includeMissing {
		missing, err := missingRequirements(ctx, db, orgID, plugin.ID)
		if err != nil {
			return nil, fmt.Errorf("load missing requirements: %w", err)
		}
		missingObjs := make([]map[string]any, 0, len(missing))
		for _, req := range missing {
			missingObjs = append(missingObjs, map[string]any{
				"provider": req.Provider,
				"kind":     req.Kind,
				"required": req.Required,
			})
		}
		obj["missing_requirements"] = missingObjs
	}
	return obj, nil
}

// --- create_agent ------------------------------------------------------------

// --- list_agents -------------------------------------------------------------

func registerListAgents(server *mcp.Server, db *gorm.DB, token *model.Token) {
	server.AddTool(&mcp.Tool{
		Name:        toolListAgents,
		Description: "List the top-level agents in this organization (id, name, description, model, status). Use get_agent to inspect one before calling update_agent.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties":           map[string]any{},
		},
	}, func(ctx context.Context, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		return handleListAgents(ctx, db, token)
	})
}

func handleListAgents(ctx context.Context, db *gorm.DB, token *model.Token) (*mcp.CallToolResult, error) {
	var rows []model.Agent
	if err := db.WithContext(ctx).
		Where("org_id = ? AND parent_agent_id IS NULL AND status <> ?", token.OrgID, "archived").
		Order("is_default DESC, name ASC").
		Find(&rows).Error; err != nil {
		return toolError("failed to list agents: " + err.Error()), nil
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		a := &rows[i]
		out = append(out, map[string]any{
			"id":          a.ID.String(),
			"name":        a.Name,
			"description": derefString(a.Description),
			"model":       a.Model,
			"status":      a.Status,
			"is_default":  a.IsDefault,
		})
	}
	return toolJSON(map[string]any{"agents": out})
}

// --- get_agent ---------------------------------------------------------------

type getAgentArgs struct {
	AgentID string `json:"agent_id"`
}

func registerGetAgent(server *mcp.Server, db *gorm.DB, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolGetAgent,
		Description: "Get the full configuration of one agent in this organization: instructions, model, enabled plugins, skills, tools, and sub-agents. Use this to inspect an agent before calling update_agent.",
		InputSchema: map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"agent_id": map[string]any{"type": "string", "description": "UUID of the agent to fetch."},
			},
			"required": []string{"agent_id"},
		},
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args getAgentArgs
		if errResult := decodeArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleGetAgent(ctx, db, token, frontendURL, args)
	})
}

func handleGetAgent(ctx context.Context, db *gorm.DB, token *model.Token, frontendURL string, args getAgentArgs) (*mcp.CallToolResult, error) {
	agentID, err := uuid.Parse(strings.TrimSpace(args.AgentID))
	if err != nil || agentID == uuid.Nil {
		return toolError("agent_id must be a valid UUID"), nil
	}
	agent, err := loadOrgAgent(ctx, db, token.OrgID, agentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return toolError("agent not found"), nil
		}
		return toolError("failed to load agent: " + err.Error()), nil
	}
	pluginSlugs, err := agentPluginSlugs(ctx, db, agent)
	if err != nil {
		return toolError(err.Error()), nil
	}
	subs, err := agentSubAgentsDetailed(ctx, db, agent.ID)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return toolJSON(map[string]any{
		"agent": map[string]any{
			"id":           agent.ID.String(),
			"name":         agent.Name,
			"description":  derefString(agent.Description),
			"instructions": derefString(agent.Instructions),
			"model":        agent.Model,
			"status":       agent.Status,
			"is_default":   agent.IsDefault,
			"plugins":      pluginSlugs,
			"skills":       agentSkillSlugs(agent),
			"tools":        agentToolIDs(agent),
			"sub_agents":   subs,
		},
		"url": agentURL(frontendURL, agent.ID),
	})
}

// agentSubAgentsDetailed returns sub-agents with the fields needed to inspect
// and re-send them via update_agent (name, description, instructions, model,
// skills, tools).
func agentSubAgentsDetailed(ctx context.Context, db *gorm.DB, parentID uuid.UUID) ([]map[string]any, error) {
	var rows []model.Agent
	if err := db.WithContext(ctx).
		Where("parent_agent_id = ? AND type = ? AND status <> ?", parentID, model.AgentTypeSubAgent, "archived").
		Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load sub-agents: %w", err)
	}
	out := make([]map[string]any, 0, len(rows))
	for i := range rows {
		r := &rows[i]
		out = append(out, map[string]any{
			"id":           r.ID.String(),
			"name":         r.Name,
			"description":  derefString(r.Description),
			"instructions": derefString(r.Instructions),
			"model":        r.Model,
			"skills":       agentSkillSlugs(r),
			"tools":        agentToolIDs(r),
		})
	}
	return out, nil
}

func derefString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

type createAgentArgs struct {
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Instructions string             `json:"instructions"`
	Model        string             `json:"model"`
	PluginSlugs  []string           `json:"plugin_slugs"`
	Skills       []string           `json:"skills"`
	Tools        []string           `json:"tools"`
	SubAgents    []subAgentToolArgs `json:"sub_agents"`
}

type subAgentToolArgs struct {
	Name         string   `json:"name"`
	Description  string   `json:"description"`
	Instructions string   `json:"instructions"`
	Skills       []string `json:"skills"`
	Tools        []string `json:"tools"`
}

func registerCreateAgent(server *mcp.Server, deps Deps, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolCreateAgent,
		Description: "Create a new agent for this organization. Core sandbox and skill tools are granted automatically; only pass optional capabilities in `tools`. Grant the parent skills, optionally pick a model (defaults to the org default), and optionally define sub-agents. Use list_org_plugins to discover valid plugin_slugs and skills.",
		InputSchema: createAgentSchema(deps.Models),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args createAgentArgs
		if errResult := decodeArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleCreateAgent(ctx, deps, token, frontendURL, args)
	})
}

func handleCreateAgent(ctx context.Context, deps Deps, token *model.Token, frontendURL string, args createAgentArgs) (*mcp.CallToolResult, error) {
	if strings.TrimSpace(args.Name) == "" {
		return toolError("name is required"), nil
	}
	if errResult := checkModelChoice(deps, args.Model); errResult != nil {
		return errResult, nil
	}
	runtime, mcpAllow, err := SplitTools(args.Tools)
	if err != nil {
		return toolError(err.Error()), nil
	}
	// Baseline sandbox tools are always granted to a top-level agent; the parent
	// schema only exposes the optional capabilities.
	mergeBaselineRuntime(runtime)
	if len(args.SubAgents) > 0 {
		runtime["subagent_task"] = true
	}
	skillSlugs, err := validateSkillSlugs(ctx, deps.DB, token.OrgID, args.Skills)
	if err != nil {
		return toolError(err.Error()), nil
	}
	plugins, err := resolvePluginSlugs(ctx, deps.DB, token.OrgID, args.PluginSlugs)
	if err != nil {
		return toolError(err.Error()), nil
	}
	subAgents, errResult := buildSubAgentToolInputs(ctx, deps, token.OrgID, args.SubAgents)
	if errResult != nil {
		return errResult, nil
	}

	in := CreateInput{
		Name:          args.Name,
		Description:   args.Description,
		Instructions:  args.Instructions,
		Model:         strings.TrimSpace(args.Model),
		Tools:         runtime,
		McpToolFilter: parentDenyFilter(mcpAllow),
		Skills:        skillsJSON(skillSlugs),
		PluginIDs:     pluginIDs(plugins),
		SubAgents:     subAgents,
	}
	agent, err := CreateAgent(ctx, deps, token.OrgID, in)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return agentResultJSON(ctx, deps.DB, agent, frontendURL, plugins, skillSlugs, runtime, mcpAllow)
}

// --- update_agent ------------------------------------------------------------

type updateAgentArgs struct {
	AgentID      string              `json:"agent_id"`
	Name         *string             `json:"name"`
	Description  *string             `json:"description"`
	Instructions *string             `json:"instructions"`
	Model        *string             `json:"model"`
	Status       *string             `json:"status"`
	PluginSlugs  *[]string           `json:"plugin_slugs"`
	Skills       *[]string           `json:"skills"`
	Tools        *[]string           `json:"tools"`
	SubAgents    *[]subAgentToolArgs `json:"sub_agents"`
}

func registerUpdateAgent(server *mcp.Server, deps Deps, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolUpdateAgent,
		Description: "Update an existing agent owned by this organization. This is a true patch: only provided fields change. A provided array (plugin_slugs, skills, tools, sub_agents) REPLACES that field entirely. Core sandbox and skill tools are granted automatically; only pass optional capabilities in `tools`. Use list_org_plugins to discover valid plugin_slugs and skills.",
		InputSchema: updateAgentSchema(deps.Models),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args updateAgentArgs
		if errResult := decodeArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleUpdateAgent(ctx, deps, token, frontendURL, args)
	})
}

func handleUpdateAgent(ctx context.Context, deps Deps, token *model.Token, frontendURL string, args updateAgentArgs) (*mcp.CallToolResult, error) {
	agentID, err := uuid.Parse(strings.TrimSpace(args.AgentID))
	if err != nil || agentID == uuid.Nil {
		return toolError("agent_id must be a valid UUID"), nil
	}

	if args.Model != nil {
		if errResult := checkModelChoice(deps, *args.Model); errResult != nil {
			return errResult, nil
		}
	}

	in := UpdateInput{
		Name:         args.Name,
		Description:  args.Description,
		Instructions: args.Instructions,
		Model:        args.Model,
		Status:       args.Status,
	}

	var (
		plugins    []model.Plugin
		skillSlugs []string
		runtime    model.JSON
		mcpAllow   []string
	)

	if args.Tools != nil {
		runtime, mcpAllow, err = SplitTools(*args.Tools)
		if err != nil {
			return toolError(err.Error()), nil
		}
		// Baseline sandbox tools are always granted to a top-level agent.
		mergeBaselineRuntime(runtime)
		// Keep subagent_task when the agent still dispatches to sub-agents: either
		// this update provides a non-empty sub_agents array, or it leaves
		// sub_agents untouched and active sub-agent rows already exist.
		keepSubagentTask := false
		if args.SubAgents != nil {
			keepSubagentTask = len(*args.SubAgents) > 0
		} else {
			keepSubagentTask, err = hasActiveSubAgents(ctx, deps.DB, agentID)
			if err != nil {
				return toolError(err.Error()), nil
			}
		}
		if keepSubagentTask {
			runtime["subagent_task"] = true
		}
		in.Tools = &runtime
		in.McpToolFilter = parentDenyFilter(mcpAllow)
		in.SetMcpFilter = true
	}
	if args.Skills != nil {
		skillSlugs, err = validateSkillSlugs(ctx, deps.DB, token.OrgID, *args.Skills)
		if err != nil {
			return toolError(err.Error()), nil
		}
		s := skillsJSON(skillSlugs)
		in.Skills = &s
	}
	if args.PluginSlugs != nil {
		plugins, err = resolvePluginSlugs(ctx, deps.DB, token.OrgID, *args.PluginSlugs)
		if err != nil {
			return toolError(err.Error()), nil
		}
		in.SetPlugins = true
		in.PluginIDs = pluginIDs(plugins)
	}
	if args.SubAgents != nil {
		subAgents, errResult := buildSubAgentToolInputs(ctx, deps, token.OrgID, *args.SubAgents)
		if errResult != nil {
			return errResult, nil
		}
		in.SubAgents = &subAgents
	}

	agent, err := UpdateAgent(ctx, deps, token.OrgID, agentID, in)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return agentResultJSON(ctx, deps.DB, agent, frontendURL, plugins, skillSlugs, runtime, mcpAllow)
}

// buildSubAgentToolInputs routes each sub-agent's tools/skills through the same
// strict validation and returns SubAgentInput rows. Errors are returned as MCP
// error results so the agent sees the guidance.
func buildSubAgentToolInputs(ctx context.Context, deps Deps, orgID uuid.UUID, args []subAgentToolArgs) ([]SubAgentInput, *mcp.CallToolResult) {
	out := make([]SubAgentInput, 0, len(args))
	for _, sub := range args {
		if strings.TrimSpace(sub.Name) == "" {
			return nil, toolError("sub-agent name is required")
		}
		runtime, mcpAllow, err := SplitTools(sub.Tools)
		if err != nil {
			return nil, toolError(fmt.Sprintf("sub-agent %q: %s", sub.Name, err.Error()))
		}
		// A sub-agent that picked nothing defaults to a read-only sandbox set so it
		// is still useful without inheriting the parent's full tool grant.
		if len(runtime) == 0 && len(mcpAllow) == 0 {
			runtime = model.JSON{"read_file": true, "glob": true, "grep": true, "file_search": true}
		}
		// A non-empty allow list keeps allow-list semantics but must never lock the
		// sub-agent out of the read-only MCP floor.
		if len(mcpAllow) > 0 {
			mcpAllow = unionReadOnlyFloor(mcpAllow)
		}
		skillSlugs, err := validateSkillSlugs(ctx, deps.DB, orgID, sub.Skills)
		if err != nil {
			return nil, toolError(fmt.Sprintf("sub-agent %q: %s", sub.Name, err.Error()))
		}
		out = append(out, SubAgentInput{
			Name:         sub.Name,
			Description:  sub.Description,
			Instructions: sub.Instructions,
			Tools:        runtime,
			McpAllow:     mcpAllow,
			Skills:       skillsJSON(skillSlugs),
		})
	}
	return out, nil
}

// --- schemas -----------------------------------------------------------------

// parentToolsArraySchema is the `tools` schema for a top-level agent. Its enum
// is only the optional capabilities: baseline sandbox tools and the read-only
// skill/channel tools are always granted and must not be listed here.
func parentToolsArraySchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Additional capabilities for the agent. Core sandbox tools (bash, file read/write/edit, search, planning) and the read-only skill/channel tools are ALWAYS granted to top-level agents and must not be listed here. subagent_task is granted automatically when sub_agents are defined.",
		"items": map[string]any{
			"type": "string",
			"enum": enumValues(ParentAssignableToolIDs()),
		},
	}
}

// subAgentToolsArraySchema is the `tools` schema for a sub-agent. Its enum is the
// full union so a read-only sub-agent can be expressed by listing just the
// read-only file tools. Sub-agents receive ONLY the tools listed.
func subAgentToolsArraySchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Tools the sub-agent may use. A sub-agent gets ONLY what is listed here — e.g. a read-only sub-agent lists just read_file, glob, grep, file_search. An empty list defaults to that read-only set. Each value must be one of the listed runtime or MCP tools.",
		"items": map[string]any{
			"type": "string",
			"enum": toolEnumValues(),
		},
	}
}

func skillsArraySchema(desc string) map[string]any {
	return map[string]any{
		"type":        "array",
		"description": desc,
		"items":       map[string]any{"type": "string"},
	}
}

func subAgentsSchema() map[string]any {
	return map[string]any{
		"type":        "array",
		"description": "Sub-agents the parent can dispatch to via subagent_task.",
		"items": map[string]any{
			"type":                 "object",
			"additionalProperties": false,
			"properties": map[string]any{
				"name":         map[string]any{"type": "string"},
				"description":  map[string]any{"type": "string"},
				"instructions": map[string]any{"type": "string"},
				"skills":       skillsArraySchema("Skill slugs the sub-agent can use."),
				"tools":        subAgentToolsArraySchema(),
			},
			"required": []string{"name"},
		},
	}
}

// modelSchema returns the schema for the optional `model` field. When the model
// catalog is available it is a strict enum so agents cannot hallucinate model
// ids; otherwise a plain string.
func modelSchema(models []string, desc string) map[string]any {
	out := map[string]any{"type": "string", "description": desc}
	if len(models) > 0 {
		enum := make([]any, len(models))
		for i, id := range models {
			enum[i] = id
		}
		out["enum"] = enum
	}
	return out
}

func createAgentSchema(models []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"name":         map[string]any{"type": "string", "description": "The agent's display name."},
			"description":  map[string]any{"type": "string", "description": "Short description of the agent."},
			"instructions": map[string]any{"type": "string", "description": "System instructions for the agent."},
			"model":        modelSchema(models, "Model the agent uses. Omit to use the org default."),
			"plugin_slugs": skillsArraySchema("Slugs of org-installed plugins to enable on the agent."),
			"skills":       skillsArraySchema("Skill slugs the parent agent can use."),
			"tools":        parentToolsArraySchema(),
			"sub_agents":   subAgentsSchema(),
		},
		"required": []string{"name"},
	}
}

func updateAgentSchema(models []string) map[string]any {
	return map[string]any{
		"type":                 "object",
		"additionalProperties": false,
		"properties": map[string]any{
			"agent_id":     map[string]any{"type": "string", "description": "UUID of the agent to update."},
			"name":         map[string]any{"type": "string", "description": "The agent's display name."},
			"description":  map[string]any{"type": "string", "description": "Short description of the agent."},
			"instructions": map[string]any{"type": "string", "description": "System instructions for the agent."},
			"model":        modelSchema(models, "Model the agent uses. Omit to leave unchanged."),
			"status":       map[string]any{"type": "string", "enum": []any{"active", "archived"}, "description": "Agent status."},
			"plugin_slugs": skillsArraySchema("Slugs of org-installed plugins to enable on the agent. Replaces the current set."),
			"skills":       skillsArraySchema("Skill slugs the parent agent can use. Replaces the current set."),
			"tools":        parentToolsArraySchema(),
			"sub_agents":   subAgentsSchema(),
		},
		"required": []string{"agent_id"},
	}
}

// checkModelChoice rejects a model id that is not in the assignable catalog,
// returning a helpful IsError result listing the allowed models. An empty model
// is allowed (create defaults to the org default; update leaves it unchanged).
func checkModelChoice(deps Deps, modelID string) *mcp.CallToolResult {
	id := strings.TrimSpace(modelID)
	if id == "" || len(deps.Models) == 0 {
		return nil
	}
	for _, allowed := range deps.Models {
		if allowed == id {
			return nil
		}
	}
	return toolError(fmt.Sprintf("unknown model %q: allowed models are: %s", id, strings.Join(deps.Models, ", ")))
}

// --- output ------------------------------------------------------------------

func agentResultJSON(ctx context.Context, db *gorm.DB, agent *model.Agent, frontendURL string, plugins []model.Plugin, skillSlugs []string, runtime model.JSON, mcpAllow []string) (*mcp.CallToolResult, error) {
	// Load the persisted plugin/skill/tool state so update responses reflect the
	// stored values, not just the request.
	pluginSlugs, err := agentPluginSlugs(ctx, db, agent)
	if err != nil {
		return toolError(err.Error()), nil
	}
	skillsOut := agentSkillSlugs(agent)
	toolsOut := agentToolIDs(agent)
	subs, err := agentSubAgents(ctx, db, agent.ID)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return toolJSON(map[string]any{
		"agent": map[string]any{
			"id":         agent.ID.String(),
			"name":       agent.Name,
			"status":     agent.Status,
			"model":      agent.Model,
			"plugins":    pluginSlugs,
			"skills":     skillsOut,
			"tools":      toolsOut,
			"sub_agents": subs,
		},
		"url": agentURL(frontendURL, agent.ID),
	})
}

func agentPluginSlugs(ctx context.Context, db *gorm.DB, agent *model.Agent) ([]string, error) {
	var pluginIDs []uuid.UUID
	if err := db.WithContext(ctx).Model(&model.AgentPluginInstall{}).
		Where("agent_id = ?", agent.ID).Pluck("plugin_id", &pluginIDs).Error; err != nil {
		return nil, fmt.Errorf("load agent plugins: %w", err)
	}
	if len(pluginIDs) == 0 {
		return []string{}, nil
	}
	var slugs []string
	if err := db.WithContext(ctx).Model(&model.Plugin{}).
		Where("id IN ?", pluginIDs).Order("slug ASC").Pluck("slug", &slugs).Error; err != nil {
		return nil, fmt.Errorf("load plugin slugs: %w", err)
	}
	if slugs == nil {
		slugs = []string{}
	}
	return slugs, nil
}

func agentSkillSlugs(agent *model.Agent) []string {
	filter := skillFilterAllow(agent.Skills)
	if filter == nil {
		return []string{}
	}
	sort.Strings(filter)
	return filter
}

// agentToolIDs reports the tool ids to echo back to the agent-builder, made
// round-trip-safe against the schema for the agent's kind. For a sub-agent
// (full enum) it reports every enabled runtime tool plus the filter allow list.
// For a parent it reports only parent-assignable ids: the optional runtime tools
// actually enabled (baseline and subagent_task are auto-granted and omitted) and
// the parent-assignable MCP grants derived from the filter — so the returned
// list can be re-sent to update_agent without tripping the parent enum.
func agentToolIDs(agent *model.Agent) []string {
	if agent.Type == model.AgentTypeSubAgent {
		return subAgentToolIDs(agent)
	}
	return parentToolIDs(agent)
}

func subAgentToolIDs(agent *model.Agent) []string {
	out := make([]string, 0, len(agent.Tools))
	for id, enabled := range agent.Tools {
		if b, ok := enabled.(bool); ok && b {
			out = append(out, id)
		}
	}
	if agent.McpToolFilter != nil {
		out = append(out, agent.McpToolFilter.Allow...)
	}
	sort.Strings(out)
	return out
}

func parentToolIDs(agent *model.Agent) []string {
	out := make([]string, 0)
	// Optional runtime tools only (e.g. lsp); baseline + subagent_task omitted.
	for id, enabled := range agent.Tools {
		if b, ok := enabled.(bool); ok && b && optionalRuntimeToolSet[id] {
			out = append(out, id)
		}
	}
	out = append(out, parentGrantedMCPTools(agent.McpToolFilter)...)
	sort.Strings(out)
	return out
}

// parentGrantedMCPTools derives the parent-assignable MCP tools a parent agent
// currently has, from its filter: nil filter → all parent-assignable MCP tools;
// a deny-list → parent-assignable MCP minus the denied ids; a legacy allow-list
// → the allow ids that are parent-assignable.
func parentGrantedMCPTools(filter *model.ToolFilter) []string {
	assignable := parentAssignableMCPTools()
	if filter == nil {
		return assignable
	}
	if len(filter.Deny) > 0 {
		denied := stringSet(filter.Deny)
		out := make([]string, 0, len(assignable))
		for _, id := range assignable {
			if !denied[id] {
				out = append(out, id)
			}
		}
		return out
	}
	if len(filter.Allow) > 0 {
		out := make([]string, 0, len(filter.Allow))
		for _, id := range filter.Allow {
			if parentAssignableMCPToolSet[id] {
				out = append(out, id)
			}
		}
		return out
	}
	// Empty filter (no allow, no deny) grants all MCP tools.
	return assignable
}

func agentSubAgents(ctx context.Context, db *gorm.DB, parentID uuid.UUID) ([]map[string]any, error) {
	var rows []model.Agent
	if err := db.WithContext(ctx).
		Where("parent_agent_id = ? AND type = ? AND status <> ?", parentID, model.AgentTypeSubAgent, "archived").
		Order("created_at ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("load sub-agents: %w", err)
	}
	out := make([]map[string]any, 0, len(rows))
	for _, row := range rows {
		out = append(out, map[string]any{"id": row.ID.String(), "name": row.Name})
	}
	return out, nil
}

// skillFilterAllow extracts the allow list from the agent Skills jsonb using
// the {"skill_filter":{"allow":[...]}} shape (with a top-level "allow"
// fallback), matching skills.skillFilterFromAgentSkills.
func skillFilterAllow(skills model.JSON) []string {
	if len(skills) == 0 {
		return nil
	}
	if raw, ok := skills["skill_filter"]; ok {
		return allowListFromAny(raw)
	}
	if _, ok := skills["allow"]; ok {
		return allowListFromAny(map[string]any(skills))
	}
	return nil
}

func allowListFromAny(raw any) []string {
	body, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var payload struct {
		Allow []string `json:"allow"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		return nil
	}
	return payload.Allow
}

func agentURL(frontendURL string, agentID uuid.UUID) string {
	base := strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	return base + "/w/settings/agents/edit/" + agentID.String()
}

// pluginInstallURL is the app page where a user installs/connects a plugin.
func pluginInstallURL(frontendURL, slug string) string {
	base := strings.TrimRight(strings.TrimSpace(frontendURL), "/")
	return base + "/w/plugins/" + slug
}

// --- helpers -----------------------------------------------------------------

func allowFilter(allow []string) *model.ToolFilter {
	if len(allow) == 0 {
		return nil
	}
	return &model.ToolFilter{Allow: allow}
}

// mergeBaselineRuntime unions the always-granted baseline sandbox tools into a
// parent agent's runtime tool map so a top-level agent can never end up without
// the core sandbox tools (the incident: an agent that picked only skill tools
// got an empty runtime map and zero built-ins).
func mergeBaselineRuntime(runtime model.JSON) {
	for _, id := range model.BaselineRuntimeToolIDs {
		runtime[id] = true
	}
}

// parentDenyFilter builds the MCP filter for a parent agent as a DENY list: the
// parent-assignable MCP tools the builder did NOT grant. A deny-list keeps
// plugin-gated MCP tools (sheets etc.) and the read-only floor
// (skills_list/skill_view/list_channels) usable while still restricting the
// optional capabilities the builder chose to withhold; an allow-list would
// silently lock out tools that are not even in the parent enum (the incident).
// Returns nil when nothing needs denying (every parent-assignable MCP tool was
// granted), which means "all MCP tools allowed".
func parentDenyFilter(mcpAllow []string) *model.ToolFilter {
	picked := stringSet(mcpAllow)
	deny := make([]string, 0)
	for _, id := range parentAssignableMCPTools() {
		if !picked[id] {
			deny = append(deny, id)
		}
	}
	if len(deny) == 0 {
		return nil
	}
	sort.Strings(deny)
	return &model.ToolFilter{Deny: deny}
}

// unionReadOnlyFloor adds the read-only MCP floor to a non-empty sub-agent allow
// list so an allow-listed sub-agent never loses access to the skill/channel
// tools. The result is deduped and sorted.
func unionReadOnlyFloor(allow []string) []string {
	seen := stringSet(allow)
	out := append([]string(nil), allow...)
	for _, id := range model.ReadOnlyMCPToolFloor {
		if !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	sort.Strings(out)
	return out
}

// hasActiveSubAgents reports whether the parent agent still has active sub-agent
// rows, used to decide whether a tools-only update should keep subagent_task.
func hasActiveSubAgents(ctx context.Context, db *gorm.DB, parentID uuid.UUID) (bool, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&model.Agent{}).
		Where("parent_agent_id = ? AND type = ? AND status = ?", parentID, model.AgentTypeSubAgent, "active").
		Count(&count).Error; err != nil {
		return false, fmt.Errorf("count sub-agents: %w", err)
	}
	return count > 0, nil
}

func pluginIDs(plugins []model.Plugin) []uuid.UUID {
	out := make([]uuid.UUID, 0, len(plugins))
	for _, plugin := range plugins {
		out = append(out, plugin.ID)
	}
	return out
}

func loadOrgAgent(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (*model.Agent, error) {
	var agent model.Agent
	if err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").
		First(&agent).Error; err != nil {
		return nil, err
	}
	return &agent, nil
}

func agentProxyToken(token *model.Token) bool {
	if token == nil || token.Meta == nil {
		return false
	}
	tokenType, _ := token.Meta[model.TokenMetaType].(string)
	return tokenType == model.TokenTypeAgentProxy
}

func tokenAgentID(token *model.Token) (uuid.UUID, error) {
	agentIDText, _ := token.Meta[model.TokenMetaAgentID].(string)
	agentID, err := uuid.Parse(strings.TrimSpace(agentIDText))
	if err != nil || agentID == uuid.Nil {
		return uuid.Nil, fmt.Errorf("agent proxy token is missing agent_id")
	}
	return agentID, nil
}

func decodeArgs(req *mcp.CallToolRequest, dst any) *mcp.CallToolResult {
	if req == nil || req.Params.Arguments == nil {
		return nil // no arguments is valid for optional-only payloads
	}
	if err := json.Unmarshal(req.Params.Arguments, dst); err != nil {
		return toolError("invalid arguments")
	}
	return nil
}

func toolError(msg string) *mcp.CallToolResult {
	return &mcp.CallToolResult{
		Content: []mcp.Content{&mcp.TextContent{Text: "Error: " + msg}},
		IsError: true,
	}
}

func toolJSON(v any) (*mcp.CallToolResult, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return toolError("failed to serialize response"), nil
	}
	return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: string(b)}}}, nil
}
