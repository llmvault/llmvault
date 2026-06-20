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
			Token:           agentRuntimeE2EToken,
			SubagentPhrases: agentRuntimeE2EAllSubagentMarkers(),
			TestCommand:     "python3 -m unittest -v",
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
        self.assertEqual(calc.helper_phrase(), %q)

if __name__ == "__main__":
    unittest.main()
`, agentRuntimeE2EToken, agentRuntimeE2ESubagentMarkers["codebase-explorer"])
	if err := os.WriteFile(filepath.Join(workspaceRoot, "calc.py"), []byte(calc), 0o600); err != nil {
		t.Fatalf("write calc.py: %v", err)
	}
	trace.Body("fixture-files", filepath.Join(workspaceRoot, "calc.py"), []byte(calc))
	if err := os.WriteFile(filepath.Join(workspaceRoot, "test_calc.py"), []byte(testFile), 0o600); err != nil {
		t.Fatalf("write test_calc.py: %v", err)
	}
	trace.Body("fixture-files", filepath.Join(workspaceRoot, "test_calc.py"), []byte(testFile))
	writeLSPFixtureProject(t, trace, workspaceRoot)
}

func writeLSPFixtureProject(t *testing.T, trace *agentRuntimeE2ETrace, workspaceRoot string) {
	t.Helper()
	files := map[string]string{
		"lsp-fixtures/deno/deno.json": `{"compilerOptions":{"strict":true}}
`,
		"lsp-fixtures/deno/mod.ts": `export function denoFixture(name: string): string {
  return ` + "`hello ${name}`" + `
}
`,
		"lsp-fixtures/typescript/package.json": `{"private":true,"dependencies":{"typescript":"latest"}}
`,
		"lsp-fixtures/typescript/tsconfig.json": `{"compilerOptions":{"strict":true,"target":"ES2022","module":"ESNext"}}
`,
		"lsp-fixtures/typescript/src/app.ts": `export function tsFixture(value: number): number {
  return value + 1
}
`,
		"lsp-fixtures/python/pyproject.toml": `[project]
name = "hivy-lsp-python-fixture"
version = "0.0.0"
`,
		"lsp-fixtures/python/app.py": `def python_fixture(name: str) -> str:
    return f"hello {name}"
`,
		"lsp-fixtures/go/go.mod": `module example.com/hivy/lspfixture

go 1.22
`,
		"lsp-fixtures/go/main.go": `package main

func goFixture(name string) string {
	return "hello " + name
}
`,
		"lsp-fixtures/rust/Cargo.toml": `[package]
name = "hivy_lsp_rust_fixture"
version = "0.0.0"
edition = "2021"
`,
		"lsp-fixtures/rust/src/lib.rs": `pub fn rust_fixture(name: &str) -> String {
    format!("hello {name}")
}
`,
		"lsp-fixtures/cpp/compile_flags.txt": `-std=c++17
`,
		"lsp-fixtures/cpp/main.cpp": `#include <string>

std::string cpp_fixture(const std::string& name) {
  return "hello " + name;
}
`,
		"lsp-fixtures/json/package.json": `{"private":true,"name":"hivy-lsp-json-fixture","version":"0.0.0"}
`,
		"lsp-fixtures/yaml/config.yaml": `service:
  name: hivy-lsp-yaml-fixture
  enabled: true
`,
		"lsp-fixtures/html/package.json": `{"private":true,"name":"hivy-lsp-html-fixture","version":"0.0.0"}
`,
		"lsp-fixtures/html/index.html": `<!doctype html>
<html>
  <body>
    <main id="fixture">HTML fixture</main>
  </body>
</html>
`,
		"lsp-fixtures/css/package.json": `{"private":true,"name":"hivy-lsp-css-fixture","version":"0.0.0"}
`,
		"lsp-fixtures/css/styles.css": `.fixture {
  color: #2563eb;
}
`,
		"lsp-fixtures/tailwind/package.json": `{"private":true,"dependencies":{"tailwindcss":"^4.0.0","typescript":"latest"}}
`,
		"lsp-fixtures/tailwind/tailwind.config.js": `module.exports = {
  content: ["./src/**/*.{ts,tsx,html}"],
  theme: { extend: {} },
}
`,
		"lsp-fixtures/tailwind/src/App.tsx": `export function App() {
  return (
    <div className="text-red-500 font-bold">
      Tailwind fixture
    </div>
  )
}
`,
		"lsp-fixtures/bash/script.sh": `#!/usr/bin/env bash
set -euo pipefail
echo "bash fixture"
`,
		"lsp-fixtures/docker/Dockerfile": `FROM scratch
LABEL name="hivy-lsp-dockerfile-fixture"
`,
	}
	for path, content := range files {
		fullPath := filepath.Join(workspaceRoot, filepath.FromSlash(path))
		if err := os.MkdirAll(filepath.Dir(fullPath), 0o700); err != nil {
			t.Fatalf("mkdir %s: %v", filepath.Dir(fullPath), err)
		}
		if err := os.WriteFile(fullPath, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", fullPath, err)
		}
		trace.Body("fixture-files", fullPath, []byte(content))
	}
}

func assertFixtureProjectCompleted(t *testing.T, trace *agentRuntimeE2ETrace, workspaceRoot string) {
	t.Helper()
	calc, err := os.ReadFile(filepath.Join(workspaceRoot, "calc.py"))
	if err != nil {
		t.Fatalf("read calc.py: %v", err)
	}
	trace.Body("assert-files", filepath.Join(workspaceRoot, "calc.py"), calc)
	content := string(calc)
	for _, want := range []string{agentRuntimeE2EToken, agentRuntimeE2ESubagentMarkers["codebase-explorer"]} {
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
	tooling, err := os.ReadFile(filepath.Join(workspaceRoot, "TOOLING_E2E.md"))
	if err != nil {
		t.Fatalf("read TOOLING_E2E.md: %v", err)
	}
	trace.Body("assert-files", filepath.Join(workspaceRoot, "TOOLING_E2E.md"), tooling)
	for _, want := range []string{agentRuntimeE2EToken, "REAL_REPOS_CONFIRMED", "FFF_TOOLS_CONFIRMED", "APPLY_PATCH_CONFIRMED", "LSP_CONFIRMED", "ALL_LSP_SERVERS_CONFIRMED"} {
		if !strings.Contains(string(tooling), want) {
			t.Fatalf("TOOLING_E2E.md missing %q:\n%s", want, tooling)
		}
	}
	for _, path := range []string{
		filepath.Join("repos", "gjson", "gjson.go"),
		filepath.Join("repos", "itsdangerous", "src", "itsdangerous", "serializer.py"),
		filepath.Join("lsp-fixtures", "deno", "mod.ts"),
		filepath.Join("lsp-fixtures", "typescript", "src", "app.ts"),
		filepath.Join("lsp-fixtures", "python", "app.py"),
		filepath.Join("lsp-fixtures", "go", "main.go"),
		filepath.Join("lsp-fixtures", "rust", "src", "lib.rs"),
		filepath.Join("lsp-fixtures", "cpp", "main.cpp"),
		filepath.Join("lsp-fixtures", "json", "package.json"),
		filepath.Join("lsp-fixtures", "yaml", "config.yaml"),
		filepath.Join("lsp-fixtures", "html", "index.html"),
		filepath.Join("lsp-fixtures", "css", "styles.css"),
		filepath.Join("lsp-fixtures", "tailwind", "src", "App.tsx"),
		filepath.Join("lsp-fixtures", "bash", "script.sh"),
		filepath.Join("lsp-fixtures", "docker", "Dockerfile"),
	} {
		if _, err := os.Stat(filepath.Join(workspaceRoot, path)); err != nil {
			t.Fatalf("expected runtime fixture file %s: %v", path, err)
		}
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
