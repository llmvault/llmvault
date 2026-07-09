package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

// --- create_agent ------------------------------------------------------------

type createAgentArgs struct {
	Name         string             `json:"name"`
	Description  string             `json:"description"`
	Instructions string             `json:"instructions"`
	Model        string             `json:"model"`
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

func registerCreateAgent(server *mcp.Server, deps Deps, token *model.Token, teamID uuid.UUID, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolCreateAgent,
		Description: "Create a new agent for this organization. Core sandbox and skill tools are granted automatically; only pass optional capabilities in `tools`. Grant the parent skills, optionally pick a model (defaults to the org default), and optionally define sub-agents. The new agent joins the calling agent's team and inherits that team's plugins (plugins are team-managed, not set per agent).",
		InputSchema: createAgentSchema(deps.Models),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if errResult := requireOrgManager(ctx, deps.DB, token.OrgID, req, "creating an agent"); errResult != nil {
			return errResult, nil
		}
		var args createAgentArgs
		if errResult := decodeArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		return handleCreateAgent(ctx, deps, token, teamID, frontendURL, args)
	})
}

func handleCreateAgent(ctx context.Context, deps Deps, token *model.Token, teamID uuid.UUID, frontendURL string, args createAgentArgs) (*mcp.CallToolResult, error) {
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
		TeamID:        teamID,
		SubAgents:     subAgents,
	}
	agent, err := CreateAgent(ctx, deps, token.OrgID, in)
	if err != nil {
		return toolError(err.Error()), nil
	}
	return agentResultJSON(ctx, deps.DB, agent, frontendURL, skillSlugs, runtime, mcpAllow)
}

// --- update_agent ------------------------------------------------------------

type updateAgentArgs struct {
	AgentID      string              `json:"agent_id"`
	Name         *string             `json:"name"`
	Description  *string             `json:"description"`
	Instructions *string             `json:"instructions"`
	Model        *string             `json:"model"`
	Status       *string             `json:"status"`
	Skills       *[]string           `json:"skills"`
	Tools        *[]string           `json:"tools"`
	SubAgents    *[]subAgentToolArgs `json:"sub_agents"`
}

func registerUpdateAgent(server *mcp.Server, deps Deps, token *model.Token, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolUpdateAgent,
		Description: "Update an existing agent owned by this organization. This is a true patch: only provided fields change. A provided array (skills, tools, sub_agents) REPLACES that field entirely. Core sandbox and skill tools are granted automatically; only pass optional capabilities in `tools`. Plugins are team-managed and cannot be set per agent. Use list_org_plugins to discover valid skills.",
		InputSchema: updateAgentSchema(deps.Models),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if errResult := requireOrgManager(ctx, deps.DB, token.OrgID, req, "changing an agent"); errResult != nil {
			return errResult, nil
		}
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
	return agentResultJSON(ctx, deps.DB, agent, frontendURL, skillSlugs, runtime, mcpAllow)
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
