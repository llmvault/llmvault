package handler

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"

	"github.com/usehivy/hivy/internal/model"
)

// subAgentInput is one sub-agent supplied when creating a user agent. Each
// becomes a row in the agents table (type='subagent') owned by the new parent.
type subAgentInput struct {
	Name          string            `json:"name"`
	Description   *string           `json:"description,omitempty"`
	Instructions  *string           `json:"instructions,omitempty"`
	Model         *string           `json:"model,omitempty"`
	Tools         *model.JSON       `json:"tools,omitempty"`
	McpToolFilter *model.ToolFilter `json:"mcp_tool_filter,omitempty"`
}

// subAgentResponse is one sub-agent as returned inside an agent payload.
type subAgentResponse struct {
	ID            string            `json:"id"`
	ParentAgentID string            `json:"parent_agent_id"`
	Name          string            `json:"name"`
	Description   *string           `json:"description,omitempty"`
	Instructions  string            `json:"instructions"`
	Model         string            `json:"model"`
	Tools         model.JSON        `json:"tools"`
	McpToolFilter *model.ToolFilter `json:"mcp_tool_filter,omitempty"`
	Status        string            `json:"status"`
	CreatedAt     string            `json:"created_at"`
	UpdatedAt     string            `json:"updated_at"`
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
		ID:            a.ID.String(),
		ParentAgentID: parent,
		Name:          a.Name,
		Description:   descPtr,
		Instructions:  instructions,
		Model:         a.Model,
		Tools:         nonNilJSON(a.Tools),
		McpToolFilter: a.McpToolFilter,
		Status:        a.Status,
		CreatedAt:     a.CreatedAt.Format(time.RFC3339),
		UpdatedAt:     a.UpdatedAt.Format(time.RFC3339),
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
func (h *AgentHandler) buildSubAgentRows(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, parentModel string, inputs *[]subAgentInput) ([]model.Agent, bool) {
	if inputs == nil || len(*inputs) == 0 {
		return nil, true
	}
	orgIDCopy := orgID
	rows := make([]model.Agent, 0, len(*inputs))
	seen := map[string]bool{}
	for _, in := range *inputs {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "sub-agent name is required"})
			return nil, false
		}
		if seen[name] {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("duplicate sub-agent name %q", name)})
			return nil, false
		}
		seen[name] = true

		subModel := cleanStringPtr(in.Model)
		if subModel == "" {
			subModel = parentModel
		} else if err := h.validateAgentSelectableModel(ctx, orgID, subModel); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("sub-agent %q: %s", name, err.Error())})
			return nil, false
		}

		tools := normalizeJSONPtr(in.Tools)
		if bad := validateToolSelectionKeys(tools); bad != "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: fmt.Sprintf("sub-agent %q: invalid tool %q", name, bad)})
			return nil, false
		}

		desc := cleanStringPtr(in.Description)
		instructions := cleanStringPtr(in.Instructions)
		rows = append(rows, model.Agent{
			OrgID:           &orgIDCopy,
			Type:            model.AgentTypeSubAgent,
			Name:            name,
			Description:     &desc,
			Instructions:    &instructions,
			Model:           subModel,
			AvailableModels: pq.StringArray{subModel},
			Tools:           tools,
			McpServers:      model.RawJSON("[]"),
			McpToolFilter:   normalizeMcpToolFilter(in.McpToolFilter),
			Skills:          model.JSON{},
			Permissions:     model.JSON{},
			Resources:       model.JSON{},
			RuntimeConfig:   model.JSON{},
			SandboxImage:    model.SandboxImageDefault,
			SandboxSize:     model.DefaultAgentSandboxSize,
			Status:          "active",
		})
	}
	return rows, true
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
