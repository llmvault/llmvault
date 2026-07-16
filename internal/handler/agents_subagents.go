package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/agents"
	"github.com/usehivy/hivy/internal/model"
)

// subAgentInput is one sub-agent supplied when creating a user agent. Each
// becomes a row in the agents table (type='subagent') owned by the new parent.
type subAgentInput struct {
	Name           string                 `json:"name"`
	Description    *string                `json:"description,omitempty"`
	Instructions   *string                `json:"instructions,omitempty"`
	Model          *string                `json:"model,omitempty"`
	Tools          *model.JSON            `json:"tools,omitempty"`
	McpToolFilter  *model.ToolFilter      `json:"mcp_tool_filter,omitempty"`
	Skills         *model.JSON            `json:"skills,omitempty"`
	AutoLoadSkills *[]model.AutoLoadSkill `json:"auto_load_skills,omitempty"`
}

// subAgentResponse is one sub-agent as returned inside an agent payload.
type subAgentResponse struct {
	ID             string               `json:"id"`
	ParentAgentID  string               `json:"parent_agent_id"`
	Name           string               `json:"name"`
	Description    *string              `json:"description,omitempty"`
	Instructions   string               `json:"instructions"`
	Model          string               `json:"model"`
	Tools          model.JSON           `json:"tools"`
	McpToolFilter  *model.ToolFilter    `json:"mcp_tool_filter,omitempty"`
	Skills         model.JSON           `json:"skills"`
	AutoLoadSkills model.AutoLoadSkills `json:"auto_load_skills"`
	Status         string               `json:"status"`
	CreatedAt      string               `json:"created_at"`
	UpdatedAt      string               `json:"updated_at"`
}

func toSubAgentResponse(a model.Agent) subAgentResponse {
	var descPtr *string
	if a.Description != nil && *a.Description != "" {
		desc := *a.Description
		descPtr = &desc
	}
	instructions := ""
	if a.Instructions != nil {
		instructions = *a.Instructions
	}
	parent := ""
	if a.ParentAgentID != nil {
		parent = a.ParentAgentID.String()
	}
	return subAgentResponse{
		ID:             a.ID.String(),
		ParentAgentID:  parent,
		Name:           a.Name,
		Description:    descPtr,
		Instructions:   instructions,
		Model:          a.Model,
		Tools:          nonNilJSON(a.Tools),
		McpToolFilter:  a.McpToolFilter,
		Skills:         nonNilJSON(a.Skills),
		AutoLoadSkills: nonNilAutoLoadSkills(a.AutoLoadSkills),
		Status:         a.Status,
		CreatedAt:      a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:      a.UpdatedAt.Format(time.RFC3339),
	}
}

// validateToolSelectionKeys returns the first key that is not a known runtime
// built-in tool id, or "" when every key is valid.
func validateToolSelectionKeys(tools model.JSON) string {
	for key := range tools {
		if !model.IsValidRuntimeBuiltInToolID(strings.TrimSpace(key)) {
			return key
		}
	}
	return ""
}

// resolveAgentToolSelection validates a requested tool map and defaults to the
// full runtime toolset when none is supplied, so a new agent is usable by
// default while still letting the caller restrict which tools it may use.
func resolveAgentToolSelection(w http.ResponseWriter, requested *model.JSON) (model.JSON, bool) {
	tools := normalizeJSONPtr(requested)
	if len(tools) == 0 {
		out := make(model.JSON, len(model.RuntimeBuiltInToolIDs))
		for _, id := range model.RuntimeBuiltInToolIDs {
			out[id] = true
		}
		return out, true
	}
	if bad := validateToolSelectionKeys(tools); bad != "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("invalid tool %q", bad)})
		return nil, false
	}
	return tools, true
}

// buildSubAgentRows validates sub-agent inputs and returns unsaved agent rows.
// ParentAgentID is left nil; the caller assigns it once the parent row exists.
// On validation failure it writes the error response and returns ok=false.
//
// Tool selection is validated here (runtime built-in ids only, matching the
// HTTP contract), then the shared agents.BuildSubAgentRows service constructs
// the rows so the sub-agent build stays in one place.
func (h *AgentHandler) buildSubAgentRows(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, parentModel string, inputs *[]subAgentInput) ([]model.Agent, bool) {
	if inputs == nil || len(*inputs) == 0 {
		return nil, true
	}
	serviceInputs := make([]agents.SubAgentInput, 0, len(*inputs))
	for _, in := range *inputs {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sub-agent name is required"})
			return nil, false
		}
		tools := normalizeJSONPtr(in.Tools)
		if bad := validateToolSelectionKeys(tools); bad != "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("sub-agent %q: invalid tool %q", name, bad)})
			return nil, false
		}
		var allow, deny []string
		if filter := normalizeMcpToolFilter(in.McpToolFilter); filter != nil {
			allow, deny = filter.Allow, filter.Deny
		}
		autoLoadSkills, ok := normalizeSubAgentAutoLoadSkillsForRequest(w, name, in.AutoLoadSkills)
		if !ok {
			return nil, false
		}
		serviceInputs = append(serviceInputs, agents.SubAgentInput{
			Name:           name,
			Description:    cleanStringPtr(in.Description),
			Instructions:   cleanStringPtr(in.Instructions),
			Model:          cleanStringPtr(in.Model),
			Tools:          tools,
			McpAllow:       allow,
			McpDeny:        deny,
			Skills:         normalizeJSONPtr(in.Skills),
			AutoLoadSkills: autoLoadSkills,
		})
	}
	rows, err := agents.BuildSubAgentRows(ctx, h.agentBuilderDeps(), orgID, parentModel, serviceInputs)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return nil, false
	}
	return rows, true
}

// agentBuilderDeps builds the agents service Deps backed by this handler's
// model validation, so the shared create/update/sub-agent logic validates
// models exactly as the HTTP handlers do.
func (h *AgentHandler) agentBuilderDeps() agents.Deps {
	return agents.Deps{
		DB:           h.db,
		DefaultModel: agentruntime.DefaultAgentModel,
		ValidateModel: func(ctx context.Context, orgID uuid.UUID, modelID string) error {
			return h.validateAgentSelectableModel(ctx, orgID, modelID)
		},
	}
}

// normalizeMcpToolFilter trims and de-dupes an MCP tool filter, returning nil
// when it carries no allow or deny entries (nil filter = all MCP tools allowed).
func normalizeMcpToolFilter(filter *model.ToolFilter) *model.ToolFilter {
	if filter == nil {
		return nil
	}
	allow := normalizeToolFilterList(filter.Allow)
	deny := normalizeToolFilterList(filter.Deny)
	if len(allow) == 0 && len(deny) == 0 {
		return nil
	}
	return &model.ToolFilter{Allow: allow, Deny: deny}
}

func normalizeConnectionMCPToolDenyForRequest(w http.ResponseWriter, value *model.ConnectionMCPToolDeny) (model.ConnectionMCPToolDeny, bool) {
	if value == nil || len(*value) == 0 {
		return model.ConnectionMCPToolDeny{}, true
	}
	out := make(model.ConnectionMCPToolDeny, len(*value))
	for connectionID, tools := range *value {
		connectionID = strings.TrimSpace(connectionID)
		parsed, err := uuid.Parse(connectionID)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "connection_mcp_tool_deny keys must be connection UUIDs"})
			return nil, false
		}
		connectionID = parsed.String()
		seen := make(map[string]bool, len(tools))
		for _, tool := range tools {
			tool = strings.TrimSpace(tool)
			if tool == "" || seen[tool] {
				continue
			}
			seen[tool] = true
			out[connectionID] = append(out[connectionID], tool)
		}
		sort.Strings(out[connectionID])
		if len(out[connectionID]) == 0 {
			delete(out, connectionID)
		}
	}
	return out, true
}

func normalizeToolFilterList(values []string) []string {
	if len(values) == 0 {
		return nil
	}
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

// loadSubAgentResponses returns a parent agent's active sub-agents for API
// responses, ordered by creation time.
func (h *AgentHandler) loadSubAgentResponses(ctx context.Context, parentID uuid.UUID) []subAgentResponse {
	var rows []model.Agent
	if err := h.db.WithContext(ctx).
		Where("parent_agent_id = ? AND type = ? AND status <> ?", parentID, model.AgentTypeSubAgent, "archived").
		Order("created_at ASC").
		Find(&rows).Error; err != nil {
		return []subAgentResponse{}
	}
	out := make([]subAgentResponse, 0, len(rows))
	for _, row := range rows {
		out = append(out, toSubAgentResponse(row))
	}
	return out
}
