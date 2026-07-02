package memory

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/usehivy/hivy/internal/model"
)

// The payloads below are copied VERBATIM from the shipped skill at
// global/plugins/service-discovery/skills/service-discovery/SKILL.md. The
// skill is the payload contract for how agents use the memory tools for
// service discovery: if a schema change breaks this test, the SKILL.md must
// change in the same commit (the test verifies the constants still appear
// verbatim in the file). Placeholder IDs ("7c9e6679-…", "b82fd4c1-…") are
// swapped for real ones at runtime; the JSON shape is untouched.
const (
	sdPayloadSearch = `{ "query": "railway services inventory", "target": { "owner": "org" }, "tags": ["service-discovery", "railway"] }`
	sdPayloadRetain = `{
  "content": "railway project acme-prod (id 9f3c2e1a): services api (id 4b7d9f21, domain api.acme.example.com), worker (id 8c1e5a37); environment production (id 2d6f8b44)",
  "target": { "owner": "org", "visibility": "this_agent" },
  "tags": ["service-discovery", "railway", "project-acme-prod"]
}`
	sdPayloadSeed = `{
  "content": "railway deploy target for acme-prod: service api (id 4b7d9f21, domain api.acme.example.com), environment production (id 2d6f8b44), project id 9f3c2e1a",
  "target": { "owner": "org", "agent_id": "7c9e6679-…" },
  "tags": ["service-discovery", "railway"]
}`
	sdPayloadForget    = `{ "memory_id": "b82fd4c1-…", "reason": "stale service inventory replaced after re-discovery" }`
	sdPayloadOrgSearch = `{ "action": "search", "query": "railway services inventory", "agent_id": "7c9e6679-…", "tags": ["service-discovery", "railway"] }`
	sdPayloadOverview  = `{ "action": "overview" }`
	sdPayloadSupForget = `{ "memory_id": "b82fd4c1-…", "reason": "duplicate railway inventory superseded by fresh discovery", "user_approved": true }`
)

func sdStrictDecode(t *testing.T, payload string, dst any) {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(payload))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		t.Fatalf("SKILL.md payload does not decode into the tool args (schema drift): %v\n%s", err, payload)
	}
}

func sdToolArgs(t *testing.T, payload, sessionID string) map[string]any {
	t.Helper()
	args := map[string]any{}
	if err := json.Unmarshal([]byte(payload), &args); err != nil {
		t.Fatalf("payload is not valid JSON: %v\n%s", err, payload)
	}
	args["_hivy_session_id"] = sessionID
	return args
}

func TestServiceDiscoverySkillPayloadContract(t *testing.T) {
	ctx := context.Background()

	// 0. Every payload constant must appear VERBATIM in the shipped SKILL.md.
	raw, err := os.ReadFile("../../global/plugins/service-discovery/skills/service-discovery/SKILL.md")
	if err != nil {
		t.Fatalf("read service-discovery SKILL.md: %v", err)
	}
	skillText := string(raw)
	for name, payload := range map[string]string{
		"search":            sdPayloadSearch,
		"retain":            sdPayloadRetain,
		"seed":              sdPayloadSeed,
		"forget":            sdPayloadForget,
		"org_search":        sdPayloadOrgSearch,
		"org_overview":      sdPayloadOverview,
		"supervisor_forget": sdPayloadSupForget,
	} {
		if !strings.Contains(skillText, payload) {
			t.Fatalf("payload %q is not present verbatim in SKILL.md — skill and contract test drifted", name)
		}
	}

	// 1. Strict schema decodes: every documented key exists on the args structs.
	sdStrictDecode(t, sdPayloadSearch, &memorySearchArgs{})
	sdStrictDecode(t, sdPayloadRetain, &memoryRetainArgs{})
	sdStrictDecode(t, sdPayloadSeed, &memoryRetainArgs{})
	sdStrictDecode(t, sdPayloadForget, &memoryForgetArgs{})
	sdStrictDecode(t, sdPayloadOrgSearch, &orgMemoriesArgs{})
	sdStrictDecode(t, sdPayloadOverview, &orgMemoriesArgs{})
	sdStrictDecode(t, sdPayloadSupForget, &memoryForgetArgs{})

	// 2. Execute the documented flow against the real tools. The fixture agent
	// is promoted to org default so the supervisor payloads (org_memories,
	// user_approved forget) execute exactly as the skill documents them.
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	if err := db.Model(&model.Agent{}).Where("id = ?", fixture.agent.ID).Update("is_default", true).Error; err != nil {
		t.Fatalf("promote fixture agent to default: %v", err)
	}
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	client := connectMemoryToolClient(t, ctx, service, fixture.token)
	sessionID := fixture.session.ID.String()

	// Search with the documented payload (must succeed; may be empty).
	callMemoryTool(t, ctx, client, "search_memories", sdToolArgs(t, sdPayloadSearch, sessionID))

	// Retain the documented self-inventory; verify the scoping the skill promises.
	retained := callMemoryTool(t, ctx, client, "retain_memory", sdToolArgs(t, sdPayloadRetain, sessionID))
	retainedID := uuid.MustParse(retained["memory"].(map[string]any)["id"].(string))
	var row model.AgentMemory
	if err := db.First(&row, "id = ?", retainedID).Error; err != nil {
		t.Fatalf("load retained memory: %v", err)
	}
	if row.Scope != model.AgentMemoryScopeOrg || row.AgentID == nil || *row.AgentID != fixture.agent.ID {
		t.Fatalf("documented retain target must yield scope=org agent_id=self, got scope=%s agent_id=%v", row.Scope, row.AgentID)
	}
	if len(row.Content) > 300 {
		t.Fatalf("documented example content exceeds the skill's own 300-char rule: %d", len(row.Content))
	}

	// Seed the documented cross-agent memory into a second org agent.
	agentB := model.Agent{ID: uuid.New(), OrgID: &fixture.org.ID, Name: "SD Contract Agent B " + uuid.NewString(), Model: "test", Status: "active"}
	if err := db.Create(&agentB).Error; err != nil {
		t.Fatalf("create agent B: %v", err)
	}
	seeded := callMemoryTool(t, ctx, client, "retain_memory",
		sdToolArgs(t, strings.ReplaceAll(sdPayloadSeed, "7c9e6679-…", agentB.ID.String()), sessionID))
	seededID := uuid.MustParse(seeded["memory"].(map[string]any)["id"].(string))
	var seededRow model.AgentMemory
	if err := db.First(&seededRow, "id = ?", seededID).Error; err != nil {
		t.Fatalf("load seeded memory: %v", err)
	}
	if seededRow.AgentID == nil || *seededRow.AgentID != agentB.ID {
		t.Fatalf("documented seed target must bind to agent B, got %v", seededRow.AgentID)
	}

	// Forget the stale inventory with the documented payload.
	callMemoryTool(t, ctx, client, "forget_memory",
		sdToolArgs(t, strings.ReplaceAll(sdPayloadForget, "b82fd4c1-…", retainedID.String()), sessionID))
	var archived model.AgentMemory
	if err := db.First(&archived, "id = ?", retainedID).Error; err != nil {
		t.Fatalf("reload forgotten memory: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("documented forget payload did not archive the memory")
	}

	// Supervisor flow: the documented org_memories payloads execute against the
	// live tool, and the seeded memory is visible with its owning agent.
	orgSearch := callMemoryTool(t, ctx, client, "org_memories",
		sdToolArgs(t, strings.ReplaceAll(sdPayloadOrgSearch, "7c9e6679-…", agentB.ID.String()), sessionID))
	if orgSearch["success"] != true {
		t.Fatalf("documented org_memories search failed: %#v", orgSearch)
	}
	overview := callMemoryTool(t, ctx, client, "org_memories", sdToolArgs(t, sdPayloadOverview, sessionID))
	agentsList, ok := overview["agents"].([]any)
	if !ok {
		t.Fatalf("documented overview payload returned no agents list: %#v", overview)
	}
	foundB := false
	for _, raw := range agentsList {
		if raw.(map[string]any)["agent_id"] == agentB.ID.String() {
			foundB = true
		}
	}
	if !foundB {
		t.Fatalf("overview does not list the seeded agent B: %#v", agentsList)
	}

	// Supervisor cleanup with the documented user_approved payload archives the
	// memory seeded into agent B.
	callMemoryTool(t, ctx, client, "forget_memory",
		sdToolArgs(t, strings.ReplaceAll(sdPayloadSupForget, "b82fd4c1-…", seededID.String()), sessionID))
	var supArchived model.AgentMemory
	if err := db.First(&supArchived, "id = ?", seededID).Error; err != nil {
		t.Fatalf("reload supervisor-forgotten memory: %v", err)
	}
	if supArchived.ArchivedAt == nil {
		t.Fatalf("documented user_approved forget did not archive the cross-agent memory")
	}
}
