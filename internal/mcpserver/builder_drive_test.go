package mcpserver

import (
	"context"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/usehivy/hivy/internal/model"
)

func TestBuildServer_RegistersDriveToolsOnlyWhenAllowed(t *testing.T) {
	tests := []struct {
		name        string
		filter      *model.ToolFilter
		wantInvoked bool
	}{
		{
			name:        "sub-agent filter excludes drive search",
			filter:      &model.ToolFilter{Allow: []string{"skill_view"}},
			wantInvoked: false,
		},
		{
			name:        "parent filter grants drive search",
			filter:      &model.ToolFilter{Allow: []string{"drive_search", "skill_view"}},
			wantInvoked: true,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			invoked := false
			_, err := BuildServer(
				context.Background(),
				&model.Token{},
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				nil,
				func(_ *mcp.Server, _ *model.Token) { invoked = true },
				tc.filter,
			)
			if err != nil {
				t.Fatalf("build server: %v", err)
			}
			if invoked != tc.wantInvoked {
				t.Fatalf("drive registration invoked = %v, want %v", invoked, tc.wantInvoked)
			}
		})
	}
}
