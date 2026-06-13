package e2e

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func TestAgentRuntimeMCPE2E(t *testing.T) {
	ctx, cancel, trace := agentRuntimeE2EContext(t, 30*time.Minute)
	defer cancel()
	trace.Logf("start", "MCP runtime E2E starting")

	alpha := newNamedMCPFixture(t, trace, "alpha", "")
	defer alpha.server.Close()
	beta := newNamedMCPFixture(t, trace, "beta", "")
	defer beta.server.Close()
	secure := newNamedMCPFixture(t, trace, "secure", "Bearer secure-mcp-token")
	defer secure.server.Close()

	scenario := startAgentRuntimeE2EScenario(
		t,
		trace,
		ctx,
		agentRuntimeE2EScenarioOptions{name: "mcp", workspaceRoot: t.TempDir()},
		func(proxyURL, controlPlaneURL, sandboxID string) map[string]any {
			servers := []any{
				map[string]any{"transport": "streamable_http", "name": "alpha", "url": containerURLForServer(alpha.server.URL), "headers": map[string]string{}},
				map[string]any{"transport": "streamable_http", "name": "beta", "url": containerURLForServer(beta.server.URL), "headers": map[string]string{}},
				map[string]any{"transport": "streamable_http", "name": "secure", "url": containerURLForServer(secure.server.URL), "headers": map[string]string{"Authorization": "Bearer ${HIVY_MCP_SECURE_TOKEN}"}},
			}
			return agentRuntimeFeatureDefinition(t, proxyURL, controlPlaneURL, sandboxID, "Runtime E2E MCP Agent", strings.Join([]string{
				"You are executing the Hivy runtime MCP E2E.",
				"Use the exact MCP tools requested by the user.",
				"The alpha, beta, and secure servers intentionally expose the same raw tool name. Use the namespaced tool names.",
			}, "\n"), agentRuntimeFileTools(), servers, []any{})
		},
	)

	response, events, responseEvents := runAgentRuntimeMessage(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, mcpE2ERequest())
	assertToolCalls(t, events, "alpha_lookup", "beta_lookup", "secure_lookup")
	assertRuntimeSessionFinal(t, trace, responseEvents, []string{"MCP_E2E_PASS", "ALPHA_OK", "BETA_OK", "SECURE_OK"})
	assertAgentRuntimePostRunAPIs(t, trace, ctx, scenario.baseURL, scenario.runtimeSecret, response.SessionID, response.TraceID, scenario.workspaceRoot)
	alpha.assertCalled(t)
	beta.assertCalled(t)
	secure.assertCalled(t)
	secure.assertHeaderSeen(t)
	scenario.proxy.assertUsed(t)
	scenario.controlPlane.waitForActivity(t)
	trace.Logf("done", "MCP runtime E2E completed")
}

type namedMCPFixture struct {
	server     *httptest.Server
	trace      *agentRuntimeE2ETrace
	name       string
	wantHeader string
	calls      atomic.Int64
	headers    atomic.Int64
}

type lookupArgs struct {
	Query string `json:"query"`
}

type lookupOutput struct {
	Server string `json:"server"`
	Query  string `json:"query"`
	Marker string `json:"marker"`
}

func newNamedMCPFixture(t *testing.T, trace *agentRuntimeE2ETrace, name, wantHeader string) *namedMCPFixture {
	t.Helper()
	fixture := &namedMCPFixture{trace: trace, name: name, wantHeader: wantHeader}
	server := mcp.NewServer(&mcp.Implementation{Name: "agent-runtime-e2e-" + name, Version: "v1"}, nil)
	mcp.AddTool(server, &mcp.Tool{Name: "lookup", Description: "Return the E2E marker for this MCP fixture."}, func(ctx context.Context, req *mcp.CallToolRequest, args lookupArgs) (*mcp.CallToolResult, lookupOutput, error) {
		fixture.calls.Add(1)
		marker := strings.ToUpper(name) + "_OK"
		fixture.trace.Logf("mcp-"+name, "lookup called query=%q marker=%s", args.Query, marker)
		out := lookupOutput{Server: name, Query: args.Query, Marker: marker}
		return &mcp.CallToolResult{Content: []mcp.Content{&mcp.TextContent{Text: fmt.Sprintf("%s:%s:%s", name, args.Query, marker)}}}, out, nil
	})
	handler := mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return server }, &mcp.StreamableHTTPOptions{
		Stateless: true, JSONResponse: true, DisableLocalhostProtection: true,
	})
	fixture.server = newPublicHTTPServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if wantHeader != "" {
			if r.Header.Get("Authorization") != wantHeader {
				http.Error(w, "missing secure MCP header", http.StatusUnauthorized)
				return
			}
			fixture.headers.Add(1)
		}
		handler.ServeHTTP(w, r)
	}))
	trace.Logf("mcp-"+name, "server listening url=%s container_url=%s", fixture.server.URL, containerURLForServer(fixture.server.URL))
	return fixture
}

func mcpE2ERequest() map[string]any {
	return map[string]any{
		"session_id": "agent-runtime-mcp-e2e",
		"user":       "agent-runtime-e2e",
		"text": strings.Join([]string{
			"Complete the MCP E2E using the exact namespaced MCP tools.",
			"1. Call alpha_lookup with query alpha-real-world.",
			"2. Call beta_lookup with query beta-real-world.",
			"3. Call secure_lookup with query secure-real-world.",
			"4. Final answer must include MCP_E2E_PASS, ALPHA_OK, BETA_OK, and SECURE_OK.",
		}, "\n"),
		"raw": map[string]any{"source": "session", "test": "agent-runtime-mcp-e2e"},
	}
}

func (f *namedMCPFixture) assertCalled(t *testing.T) {
	t.Helper()
	if calls := f.calls.Load(); calls == 0 {
		t.Fatalf("%s MCP fixture was not called", f.name)
	}
	f.trace.Logf("assert", "%s MCP fixture calls=%d", f.name, f.calls.Load())
}

func (f *namedMCPFixture) assertHeaderSeen(t *testing.T) {
	t.Helper()
	if f.wantHeader != "" && f.headers.Load() == 0 {
		t.Fatalf("%s MCP fixture did not receive expected auth header", f.name)
	}
	f.trace.Logf("assert", "%s MCP auth header count=%d", f.name, f.headers.Load())
}
