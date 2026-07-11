package agentruntime

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// The universal floor adds skill_view to every compiled filter. A deny-only
// legacy filter grants no optional capability, and an explicit deny removes a
// matching explicit grant.
func TestNormalizeToolFilter_AppliesReadOnlyFloor(t *testing.T) {
	tests := []struct {
		name      string
		in        *model.ToolFilter
		wantAllow []string
		wantDeny  []string
		wantNil   bool
	}{
		{
			name:      "allow list gains universal skill view",
			in:        &model.ToolFilter{Allow: []string{"web_search"}},
			wantAllow: []string{"skill_view", "web_search"},
			wantDeny:  nil,
		},
		{
			name:      "explicit deny removes explicit grant",
			in:        &model.ToolFilter{Allow: []string{"web_search"}, Deny: []string{"web_search"}},
			wantAllow: []string{"skill_view"},
			wantDeny:  nil,
		},
		{
			name:      "pure deny list grants no optional capability",
			in:        &model.ToolFilter{Deny: []string{"generate_image"}},
			wantAllow: []string{"skill_view"},
			wantDeny:  nil,
		},
		{
			name:      "nil filter grants only universal tool",
			in:        nil,
			wantAllow: []string{"skill_view"},
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeToolFilter(tc.in)
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
	for _, id := range model.ReadOnlyMCPToolFloor {
		if !containsString(filter.Allow, id) {
			t.Fatalf("allow = %#v, want floor id %q", filter.Allow, id)
		}
	}
	if !containsString(filter.Allow, "web_search") {
		t.Fatalf("allow = %#v, want original web_search entry", filter.Allow)
	}
}
