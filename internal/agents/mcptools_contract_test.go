package agents

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// The payloads below are copied VERBATIM from the shipped skill at
// global/plugins/agent-builder/skills/agent-builder/SKILL.md. The skill is the
// payload contract: if a schema change breaks this test, the SKILL.md must
// change in the same commit (TestAgentBuilderSkillPayloadContract verifies the
// constants still appear verbatim in the file). Placeholder values
// ("7c9e6679-…", "github", "github-triage", "MODEL_ID_FROM_ENUM") are swapped
// for real ones at runtime; the JSON shape is untouched.
const (
	abPayloadListEmpty = `{}`
	abPayloadGetAgent  = `{ "agent_id": "7c9e6679-…" }`
	abPayloadCreate    = `{
  "name": "Support Triage",
  "description": "Triages incoming support requests, drafts replies, and escalates edge cases to a human.",
  "instructions": "You are Support Triage for the team's support inbox.\n\nYour job: classify each incoming request (bug, billing, how-to, feature request), answer the ones covered by known solutions, and escalate anything ambiguous, angry, or contractual to a human — never guess on those.\n\nHow to work: read the full request first. Search the web only when the answer likely changed recently. When you draft a reply, delegate to your Responder sub-agent and review its draft before sending.\n\nBoundaries: never promise refunds, legal terms, or timelines. Never reply to legal threats — escalate.\n\nVoice: warm, direct, under 8 sentences.",
  "plugin_slugs": ["github"],
  "skills": ["github-triage"],
  "tools": ["web_search", "web_fetch"],
  "sub_agents": [
    {
      "name": "Responder",
      "description": "Drafts the customer-facing reply for requests Triage has classified.",
      "instructions": "Draft a reply for the classified support request you are given. Match the team voice: warm, direct, no filler. Return only the draft.",
      "tools": ["web_fetch"]
    }
  ]
}`
	abPayloadUpdateDescription = `{ "agent_id": "7c9e6679-…", "description": "Triages support requests for the EU team." }`
	abPayloadUpdateArchive     = `{ "agent_id": "7c9e6679-…", "status": "archived" }`
	abPayloadUpdateAddTool     = `{
  "agent_id": "7c9e6679-…",
  "tools": ["web_search", "web_fetch", "generate_image"]
}`
	abPayloadUpdateModel = `{ "agent_id": "7c9e6679-…", "model": "MODEL_ID_FROM_ENUM" }`
)

// assertBuilderStrictDecode pins skill↔schema key consistency: every key in the
// documented payload (including nested sub_agents objects) must exist on the
// tool's argument struct.
func assertBuilderStrictDecode(t *testing.T, payload string, dst any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		t.Fatalf("SKILL.md payload does not decode into the tool args (schema drift): %v\n%s", err, payload)
	}
}

func builderResultJSON(t *testing.T, res *mcp.CallToolResult) map[string]any {
	t.Helper()
	if res == nil {
		t.Fatalf("nil tool result")
	}
	if res.IsError {
		t.Fatalf("tool returned error: %s", errResultText(res))
	}
	out := map[string]any{}
	if err := json.Unmarshal([]byte(errResultText(res)), &out); err != nil {
		t.Fatalf("tool result is not JSON: %v\n%s", err, errResultText(res))
	}
	return out
}

func assertBuilderToolError(t *testing.T, res *mcp.CallToolResult, wants ...string) {
	t.Helper()
	if res == nil || !res.IsError {
		t.Fatalf("expected IsError result containing %q, got: %#v", wants, res)
	}
	text := errResultText(res)
	for _, want := range wants {
		if !strings.Contains(text, want) {
			t.Fatalf("error %q does not contain %q", text, want)
		}
	}
}

func agentToolStrings(t *testing.T, agentObj map[string]any, key string) []string {
	t.Helper()
	raw, ok := agentObj[key].([]any)
	if !ok {
		t.Fatalf("agent object has no %q array: %#v", key, agentObj)
	}
	out := make([]string, 0, len(raw))
	for _, v := range raw {
		out = append(out, v.(string))
	}
	return out
}

func containsString(list []string, want string) bool {
	for _, v := range list {
		if v == want {
			return true
		}
	}
	return false
}
