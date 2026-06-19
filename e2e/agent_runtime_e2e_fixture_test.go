package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type fixtureRequirementArgs struct {
	Detail string `json:"detail,omitempty"`
}

type fixtureRequirementOutput struct {
	Token           string   `json:"token"`
	SubagentPhrases []string `json:"subagent_phrases"`
	TestCommand     string   `json:"test_command"`
}

type agentRuntimeFixtureMCP struct {
	server *httptest.Server
	calls  atomic.Int64
	trace  *agentRuntimeE2ETrace
}

func newAgentRuntimeFixtureMCP(t *testing.T, trace *agentRuntimeE2ETrace) *agentRuntimeFixtureMCP {
	t.Helper()
	fixture := &agentRuntimeFixtureMCP{trace: trace}
	server := mcp.NewServer(&mcp.Implementation{Name: "agent-runtime-e2e-fixture", Version: "v1"}, nil)
	mcp.AddTool(server, &mcp.Tool{
		Name:        "requirements",
		Description: "Return the exact token and requirement phrases for the Hivy agent runtime E2E.",
	}, func(ctx context.Context, req *mcp.CallToolRequest, args fixtureRequirementArgs) (*mcp.CallToolResult, fixtureRequirementOutput, error) {
		fixture.calls.Add(1)
		fixture.trace.Logf("fixture-mcp", "requirements tool called detail=%q", args.Detail)
		out := fixtureRequirementOutput{
			Token: agentRuntimeE2EToken,
			SubagentPhrases: []string{
				"PLANNER_SUBAGENT_CONFIRMED",
				"QA_SUBAGENT_CONFIRMED",
				"REVIEW_SUBAGENT_CONFIRMED",
			},
			TestCommand: "python3 -m unittest -v",
		}
		return &mcp.CallToolResult{
			Content: []mcp.Content{
				&mcp.TextContent{Text: fmt.Sprintf("token=%s phrases=%s test_command=%s", out.Token, strings.Join(out.SubagentPhrases, ","), out.TestCommand)},
			},
		}, out, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server {
		return server
	}, &mcp.StreamableHTTPOptions{
		Stateless:                  true,
		JSONResponse:               true,
		DisableLocalhostProtection: true,
	})
	fixture.server = newPublicHTTPServer(t, handler)
	trace.Logf("fixture-mcp", "server listening url=%s container_url=%s", fixture.server.URL, containerURLForServer(fixture.server.URL))
	return fixture
}

func writeFixtureProject(t *testing.T, trace *agentRuntimeE2ETrace, workspaceRoot string) {
	t.Helper()
	calc := `def runtime_token():
    return "PLACEHOLDER_TOKEN"

def helper_phrase():
    return "PLACEHOLDER_HELPER"
`
	testFile := fmt.Sprintf(`import unittest
import calc

class RuntimeE2ETest(unittest.TestCase):
    def test_token_from_fixture(self):
        self.assertEqual(calc.runtime_token(), %q)

    def test_subagent_phrase_was_written(self):
        self.assertEqual(calc.helper_phrase(), "PLANNER_SUBAGENT_CONFIRMED")

if __name__ == "__main__":
    unittest.main()
`, agentRuntimeE2EToken)
	if err := os.WriteFile(filepath.Join(workspaceRoot, "calc.py"), []byte(calc), 0o600); err != nil {
		t.Fatalf("write calc.py: %v", err)
	}
	trace.Body("fixture-files", filepath.Join(workspaceRoot, "calc.py"), []byte(calc))
	if err := os.WriteFile(filepath.Join(workspaceRoot, "test_calc.py"), []byte(testFile), 0o600); err != nil {
		t.Fatalf("write test_calc.py: %v", err)
	}
	trace.Body("fixture-files", filepath.Join(workspaceRoot, "test_calc.py"), []byte(testFile))
}

func assertFixtureProjectCompleted(t *testing.T, trace *agentRuntimeE2ETrace, workspaceRoot string) {
	t.Helper()
	calc, err := os.ReadFile(filepath.Join(workspaceRoot, "calc.py"))
	if err != nil {
		t.Fatalf("read calc.py: %v", err)
	}
	trace.Body("assert-files", filepath.Join(workspaceRoot, "calc.py"), calc)
	content := string(calc)
	for _, want := range []string{agentRuntimeE2EToken, "PLANNER_SUBAGENT_CONFIRMED"} {
		if !strings.Contains(content, want) {
			t.Fatalf("calc.py missing %q:\n%s", want, content)
		}
	}
	notes, err := os.ReadFile(filepath.Join(workspaceRoot, "e2e_notes.txt"))
	if err != nil {
		t.Fatalf("read e2e_notes.txt: %v", err)
	}
	trace.Body("assert-files", filepath.Join(workspaceRoot, "e2e_notes.txt"), notes)
	if !strings.Contains(string(notes), agentRuntimeE2EToken) {
		t.Fatalf("notes file missing token:\n%s", notes)
	}
	cmd := exec.Command("python3", "-m", "unittest", "-v")
	cmd.Dir = workspaceRoot
	out, err := cmd.CombinedOutput()
	trace.Body("assert-files", "python3 -m unittest -v output", out)
	if err != nil {
		t.Fatalf("fixture unittest failed after runtime task: %v\n%s", err, out)
	}
	trace.Logf("assert-files", "fixture project assertions passed")
}
