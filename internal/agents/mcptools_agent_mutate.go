package agents

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

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
		Description: "Create a new agent for this organization. Core sandbox and skill tools are granted automatically; only pass optional capabilities in `tools`. Grant the parent skills, optionally pick a model, and optionally define sub-agents. The new agent joins the calling agent's team and receives that team's connections and skills.",
		InputSchema: createAgentSchema(deps.Models),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		if errResult := requireTeamManager(ctx, deps.DB, token.OrgID, teamID, req, "creating an agent"); errResult != nil {
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
	skillSlugs, err := validateSkillSlugs(ctx, deps.DB, token.OrgID, teamID, args.Skills)
	if err != nil {
		return toolError(err.Error()), nil
	}
	subAgents, errResult := buildSubAgentToolInputs(ctx, deps, token.OrgID, teamID, args.SubAgents)
	if errResult != nil {
		return errResult, nil
	}

	in := CreateInput{
		Name:          args.Name,
		Description:   args.Description,
		Instructions:  args.Instructions,
		Model:         strings.TrimSpace(args.Model),
		Tools:         runtime,
		McpToolFilter: parentAllowFilter(mcpAllow),
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

func registerUpdateAgent(server *mcp.Server, deps Deps, token *model.Token, teamID uuid.UUID, frontendURL string) {
	server.AddTool(&mcp.Tool{
		Name:        toolUpdateAgent,
		Description: "Update an existing agent in the calling Hivy agent's team. This is a true patch: only provided fields change. A provided array (skills, tools, sub_agents) REPLACES that field entirely. Use list_team_skills to discover valid skills.",
		InputSchema: updateAgentSchema(deps.Models),
	}, func(ctx context.Context, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
		var args updateAgentArgs
		if errResult := decodeArgs(req, &args); errResult != nil {
			return errResult, nil
		}
		agentID, err := uuid.Parse(strings.TrimSpace(args.AgentID))
		if err != nil || agentID == uuid.Nil {
			return toolError("agent_id must be a valid UUID"), nil
		}
		if errResult := authorizeUpdateTarget(ctx, deps, token.OrgID, teamID, req, agentID); errResult != nil {
			return errResult, nil
		}
		return handleUpdateAgent(ctx, deps, token, teamID, frontendURL, args)
	})
}

// authorizeUpdateTarget first binds the target to the calling Hivy agent's team,
// then applies the human team-management gate. A manager can authorize the
// action, but cannot widen Hivy's team scope.
func authorizeUpdateTarget(ctx context.Context, deps Deps, orgID, teamID uuid.UUID, req *mcp.CallToolRequest, agentID uuid.UUID) *mcp.CallToolResult {
	agent, err := loadTeamAgent(ctx, deps.DB, orgID, teamID, agentID)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return toolError(ErrAgentNotFound.Error())
		}
		return toolError("failed to load agent: " + err.Error())
	}
	return requireTeamManager(ctx, deps.DB, orgID, agent.TeamID, req, "changing an agent")
}

func handleUpdateAgent(ctx context.Context, deps Deps, token *model.Token, teamID uuid.UUID, frontendURL string, args updateAgentArgs) (*mcp.CallToolResult, error) {
	agentID, err := uuid.Parse(strings.TrimSpace(args.AgentID))
	if err != nil || agentID == uuid.Nil {
		return toolError("agent_id must be a valid UUID"), nil
	}
	target, err := loadTeamAgent(ctx, deps.DB, token.OrgID, teamID, agentID)
	if err != nil {
		return toolError(ErrAgentNotFound.Error()), nil
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
		in.McpToolFilter = parentAllowFilter(mcpAllow)
		in.SetMcpFilter = true
	}
	if args.Skills != nil {
		skillSlugs, err = validateSkillSlugs(ctx, deps.DB, token.OrgID, target.TeamID, *args.Skills)
		if err != nil {
			return toolError(err.Error()), nil
		}
		s := skillsJSON(skillSlugs)
		in.Skills = &s
	}
	if args.SubAgents != nil {
		subAgents, errResult := buildSubAgentToolInputs(ctx, deps, token.OrgID, target.TeamID, *args.SubAgents)
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
func buildSubAgentToolInputs(ctx context.Context, deps Deps, orgID, teamID uuid.UUID, args []subAgentToolArgs) ([]SubAgentInput, *mcp.CallToolResult) {
	out := make([]SubAgentInput, 0, len(args))
	for _, sub := range args {
		if strings.TrimSpace(sub.Name) == "" {
			return nil, toolError("sub-agent name is required")
		}
		runtime, mcpAllow, err := SplitTools(sub.Tools)
		if err != nil {
			return nil, toolError(fmt.Sprintf("sub-agent %q: %s", sub.Name, err.Error()))
		}
		// A sub-agent that picked nothing defaults to read_file so it is still
		// useful without inheriting the parent's full tool grant.
		if len(runtime) == 0 && len(mcpAllow) == 0 {
			runtime = model.JSON{"read_file": true}
		}
		// A non-empty allow list keeps allow-list semantics but must never lock the
		// sub-agent out of its skill-loading MCP floor.
		if len(mcpAllow) > 0 {
			mcpAllow = unionSubAgentReadOnlyFloor(mcpAllow)
		}
		skillSlugs, err := validateSkillSlugs(ctx, deps.DB, orgID, teamID, sub.Skills)
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
