package agentruntime

import (
	"context"
	"reflect"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// The read-only floor unions skill/channel tools into any non-empty allow list,
// unless a tool is explicitly denied, and leaves nil / pure deny filters alone.
func TestNormalizeToolFilter_AppliesReadOnlyFloor(t *testing.T) {
	tests := []struct {
		name      string
		in        *model.ToolFilter
		wantAllow []string
		wantDeny  []string
		wantNil   bool
	}{
		{
			name:      "allow list gains full floor",
			in:        &model.ToolFilter{Allow: []string{"skills_list"}},
			wantAllow: []string{"list_channels", "skill_view", "skills_list"},
			wantDeny:  nil,
		},
		{
			name:      "explicit deny wins over floor",
			in:        &model.ToolFilter{Allow: []string{"web_search"}, Deny: []string{"list_channels"}},
			wantAllow: []string{"skill_view", "skills_list", "web_search"},
			wantDeny:  []string{"list_channels"},
		},
		{
			name:      "pure deny list is unchanged",
			in:        &model.ToolFilter{Deny: []string{"generate_image"}},
			wantAllow: nil,
			wantDeny:  []string{"generate_image"},
		},
		{
			name:    "nil filter stays nil",
			in:      nil,
			wantNil: true,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := normalizeToolFilter(tc.in)
			if tc.wantNil {
				if got != nil {
					t.Fatalf("filter = %#v, want nil", got)
				}
				return
			}
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
		McpToolFilter: &model.ToolFilter{Allow: []string{"cron"}},
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
	if !containsString(filter.Allow, "cron") {
		t.Fatalf("allow = %#v, want original cron entry", filter.Allow)
	}
}
