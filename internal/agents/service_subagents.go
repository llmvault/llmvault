package agents

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// BuildSubAgentRows validates sub-agent inputs and returns unsaved agent rows
// (ParentAgentID left nil), so the HTTP handler and the create/update service
// share one implementation. Supports skills (Skills jsonb) in addition to
// tools and the MCP filter.
func BuildSubAgentRows(ctx context.Context, deps Deps, orgID uuid.UUID, parentModel string, inputs []SubAgentInput) ([]model.Agent, error) {
	return buildSubAgentRows(ctx, deps, orgID, parentModel, inputs)
}

// buildSubAgentRows validates sub-agent inputs and returns unsaved agent rows
// (ParentAgentID left nil). Supports skills (Skills jsonb) in addition to tools
// and the MCP filter.
func buildSubAgentRows(ctx context.Context, deps Deps, orgID uuid.UUID, parentModel string, inputs []SubAgentInput) ([]model.Agent, error) {
	if len(inputs) == 0 {
		return nil, nil
	}
	orgIDCopy := orgID
	rows := make([]model.Agent, 0, len(inputs))
	seen := map[string]bool{}
	for _, in := range inputs {
		name := strings.TrimSpace(in.Name)
		if name == "" {
			return nil, fmt.Errorf("sub-agent name is required")
		}
		if seen[name] {
			return nil, fmt.Errorf("duplicate sub-agent name %q", name)
		}
		seen[name] = true

		subModel := strings.TrimSpace(in.Model)
		if subModel == "" {
			subModel = parentModel
		} else if err := deps.validateModel(ctx, orgID, subModel); err != nil {
			return nil, fmt.Errorf("sub-agent %q: %s", name, err.Error())
		}

		tools := in.Tools
		if tools == nil {
			tools = model.JSON{}
		}
		skills := in.Skills
		if skills == nil {
			skills = model.JSON{}
		}
		desc := strings.TrimSpace(in.Description)
		instructions := strings.TrimSpace(in.Instructions)
		rows = append(rows, model.Agent{
			OrgID:          &orgIDCopy,
			Type:           model.AgentTypeSubAgent,
			Name:           name,
			Description:    &desc,
			Instructions:   &instructions,
			Model:          subModel,
			Tools:          tools,
			McpServers:     model.RawJSON("[]"),
			McpToolFilter:  subAgentFilter(in.McpAllow, in.McpDeny),
			Skills:         skills,
			AutoLoadSkills: append(model.AutoLoadSkills(nil), in.AutoLoadSkills...),
			Permissions:    model.JSON{},
			Resources:      model.JSON{},
			RuntimeConfig:  model.JSON{},
			SandboxImage:   model.SandboxImageDefault,
			SandboxSize:    model.DefaultAgentSandboxSize,
			Status:         "active",
		})
	}
	return rows, nil
}

func subAgentFilter(allow, deny []string) *model.ToolFilter {
	allow = dedupeNonEmpty(allow)
	deny = dedupeNonEmpty(deny)
	if len(allow) == 0 && len(deny) == 0 {
		return nil
	}
	return &model.ToolFilter{Allow: allow, Deny: deny}
}

func mapWriteError(err error) error {
	if isDuplicateKeyError(err) {
		return ErrDuplicateName
	}
	return err
}
