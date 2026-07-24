package agentruntime

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// The universal parent floor is added after optional grants are normalized. A
// deny-only legacy filter grants no optional capability, and an explicit deny
// removes a matching explicit grant.
func TestCompileMCPToolFilter_AppliesBaselineParentFloor(t *testing.T) {
	tests := []struct {
		name      string
		in        *model.ToolFilter
		wantAllow []string
		wantDeny  []string
		wantNil   bool
	}{
		{
			name:      "allow list gains universal parent tools",
			in:        &model.ToolFilter{Allow: []string{"web_search"}},
			wantAllow: []string{"archive_skill", "create_skill", "cron", "drive_search", "list_team_skills", "search_knowledge_base", "skill_view", "update_skill", "web_search"},
			wantDeny:  nil,
		},
		{
			name:      "explicit deny removes explicit grant",
			in:        &model.ToolFilter{Allow: []string{"web_search"}, Deny: []string{"web_search"}},
			wantAllow: []string{"archive_skill", "create_skill", "cron", "drive_search", "list_team_skills", "search_knowledge_base", "skill_view", "update_skill"},
			wantDeny:  nil,
		},
		{
			name:      "pure deny list grants no optional capability",
			in:        &model.ToolFilter{Deny: []string{"generate_image"}},
			wantAllow: []string{"archive_skill", "create_skill", "cron", "drive_search", "list_team_skills", "search_knowledge_base", "skill_view", "update_skill"},
			wantDeny:  nil,
		},
		{
			name:      "nil filter grants only universal parent tools",
			in:        nil,
			wantAllow: []string{"archive_skill", "create_skill", "cron", "drive_search", "list_team_skills", "search_knowledge_base", "skill_view", "update_skill"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := compileMCPToolFilter(tc.in)
			if got == nil {
				t.Fatalf("filter = nil, want %#v/%#v", tc.wantAllow, tc.wantDeny)
			}
			if !reflect.DeepEqual(got.Allow, tc.wantAllow) {
				t.Fatalf("allow = %#v, want %#v", got.Allow, tc.wantAllow)
			}
			if !reflect.DeepEqual(got.Deny, tc.wantDeny) {
				t.Fatalf("deny = %#v, want %#v", got.Deny, tc.wantDeny)
			}
		})
	}
}

// The floor must apply through the plain user-created agent resolution path
// (agent.McpToolFilter set, no catalog); db is unused for that branch.
func TestResolveAgentMCPToolFilter_AppliesFloorForUserAgent(t *testing.T) {
	orgID := uuid.New()
	agent := &model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		Name:          "Allow-listed",
		Model:         DefaultAgentModel,
		McpToolFilter: &model.ToolFilter{Allow: []string{"web_search"}},
	}
	filter := resolveAgentMCPToolFilter(context.Background(), nil, agent)
	if filter == nil {
		t.Fatalf("filter = nil, want floored allow list")
	}
	for _, id := range model.BaselineParentMCPToolIDs {
		if !containsString(filter.Allow, id) {
			t.Fatalf("allow = %#v, want floor id %q", filter.Allow, id)
		}
	}
	if !containsString(filter.Allow, "web_search") {
		t.Fatalf("allow = %#v, want original web_search entry", filter.Allow)
	}
}

func TestResolveAgentMCPToolFilter_DerivesEmailToolsFromInbox(t *testing.T) {
	orgID := uuid.New()
	withoutInbox := &model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		Name:          "No inbox",
		Model:         DefaultAgentModel,
		McpToolFilter: &model.ToolFilter{Allow: append([]string(nil), model.AgentEmailMCPToolIDs...)},
	}
	filter := resolveAgentMCPToolFilter(context.Background(), nil, withoutInbox)
	for _, id := range model.AgentEmailMCPToolIDs {
		if containsString(filter.Allow, id) {
			t.Fatalf("allow = %#v, must not contain %q without an inbox", filter.Allow, id)
		}
	}

	withInbox := &model.Agent{
		ID:                  uuid.New(),
		OrgID:               &orgID,
		Name:                "Provisioned inbox",
		Model:               DefaultAgentModel,
		EmailInboxLocalPart: "provisioned-agent",
		McpToolFilter:       &model.ToolFilter{},
		AgentCatalog: &model.AgentCatalog{Manifest: model.RawJSON(`{
			"mcp_tool_filter": {"allow": ["web_search"]}
		}`)},
	}
	filter = resolveAgentMCPToolFilter(context.Background(), nil, withInbox)
	if !containsString(filter.Allow, "web_search") {
		t.Fatalf("allow = %#v, want catalog grant web_search", filter.Allow)
	}
	for _, id := range model.AgentEmailMCPToolIDs {
		if !containsString(filter.Allow, id) {
			t.Fatalf("allow = %#v, want inbox-derived tool %q", filter.Allow, id)
		}
	}
}

func TestCompileSubAgentMCPToolFilter_ExcludesDriveSearch(t *testing.T) {
	filter := compileSubAgentMCPToolFilter(&model.ToolFilter{Allow: []string{"drive_search", "web_search"}}, false)
	if filter == nil {
		t.Fatal("filter = nil")
	}
	if containsString(filter.Allow, "drive_search") {
		t.Fatalf("sub-agent allow list contains drive_search: %#v", filter.Allow)
	}
	if !reflect.DeepEqual(filter.Allow, []string{"skill_view", "web_search"}) {
		t.Fatalf("allow = %#v, want skill_view+web_search", filter.Allow)
	}
}

func TestCompileSubAgentMCPToolFilter_DerivesEmailToolsFromInbox(t *testing.T) {
	filter := compileSubAgentMCPToolFilter(
		&model.ToolFilter{Allow: append([]string(nil), model.AgentEmailMCPToolIDs...)},
		false,
	)
	for _, id := range model.AgentEmailMCPToolIDs {
		if containsString(filter.Allow, id) {
			t.Fatalf("allow = %#v, must not contain %q without an inbox", filter.Allow, id)
		}
	}

	filter = compileSubAgentMCPToolFilter(&model.ToolFilter{}, true)
	for _, id := range model.AgentEmailMCPToolIDs {
		if !containsString(filter.Allow, id) {
			t.Fatalf("allow = %#v, want inbox-derived tool %q", filter.Allow, id)
		}
	}
}
