package agents

import (
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func errResultText(res *mcp.CallToolResult) string {
	if res == nil || len(res.Content) == 0 {
		return ""
	}
	if tc, ok := res.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func TestCheckModelChoice(t *testing.T) {
	deps := Deps{Models: []string{"deepseek-v4-flash", "claude-opus-4-8"}}

	if res := checkModelChoice(deps, ""); res != nil {
		t.Fatalf("empty model should be allowed, got error result")
	}
	if res := checkModelChoice(deps, "claude-opus-4-8"); res != nil {
		t.Fatalf("known model should be allowed, got error result")
	}
	res := checkModelChoice(deps, "gpt-9-turbo")
	if res == nil || !res.IsError {
		t.Fatalf("unknown model should return an IsError result, got %#v", res)
	}
	text := errResultText(res)
	if !strings.Contains(text, "gpt-9-turbo") || !strings.Contains(text, "deepseek-v4-flash") {
		t.Fatalf("error should name the bad model and list allowed models, got: %q", text)
	}
	if res := checkModelChoice(Deps{}, "anything"); res != nil {
		t.Fatalf("no catalog should skip enforcement, got error result")
	}
}
