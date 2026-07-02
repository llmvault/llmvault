package memory

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func TestMemoryMCPToolsRetainSearchAndForget(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	client := connectMemoryToolClient(t, ctx, service, fixture.token)

	tools, err := client.ListTools(ctx, nil)
	if err != nil {
		t.Fatalf("list tools: %v", err)
	}
	assertMemoryToolDescriptions(t, tools.Tools)

	retained := callMemoryTool(t, ctx, client, "retain_memory", map[string]any{
		"content":          "The agent should use the Helio launch checklist before release planning.",
		"target":           map[string]any{"owner": "user", "visibility": "this_agent"},
		"tags":             []string{"launch", "preference"},
		"_hivy_session_id": fixture.session.ID.String(),
	})
	retainedMemory := retained["memory"].(map[string]any)
	retainedID := uuid.MustParse(retainedMemory["id"].(string))
	assertRetainedMemoryTarget(t, db, retainedID, fixture.user.ID, fixture.agent.ID)

	seedReadyMemory(t, service, fixture.org.ID, nil, nil, "The org billing policy is Net 30 for annual contracts.")
	seedReadyMemory(t, service, fixture.org.ID, &fixture.agent.ID, nil, "This agent owns the Helio release checklist.")

	agentResults := callMemoryTool(t, ctx, client, "search_memories", map[string]any{
		"query":            "Helio release checklist",
		"target":           map[string]any{"owner": "org", "visibility": "this_agent"},
		"_hivy_session_id": fixture.session.ID.String(),
	})
	if got := len(agentResults["results"].([]any)); got != 1 {
		t.Fatalf("this_agent search returned %d results, want 1: %#v", got, agentResults)
	}

	sharedResults := callMemoryTool(t, ctx, client, "search_memories", map[string]any{
		"query":            "billing policy",
		"target":           map[string]any{"owner": "org", "visibility": "all_agents"},
		"_hivy_session_id": fixture.session.ID.String(),
	})
	if got := len(sharedResults["results"].([]any)); got != 1 {
		t.Fatalf("all_agents search returned %d results, want 1: %#v", got, sharedResults)
	}

	assertMemoryToolError(t, ctx, client, "search_memories", map[string]any{
		"query":            "this query has far too many words",
		"_hivy_session_id": fixture.session.ID.String(),
	}, "query must be at most")
	assertMemoryToolError(t, ctx, client, "retain_memory", map[string]any{
		"content":          "The agent should remember this malformed tag test.",
		"target":           map[string]any{"owner": "org", "visibility": "this_agent"},
		"tags":             []string{"Project Helio"},
		"_hivy_session_id": fixture.session.ID.String(),
	}, "lowercase kebab-case")

	sharedID := seedReadyMemory(t, service, fixture.org.ID, nil, nil, "Shared org memory can be forgotten by an agent.")
	callMemoryTool(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        sharedID.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	})
	assertMemoryArchived(t, db, sharedID)

	otherUserMemoryID := seedReadyMemory(t, service, fixture.org.ID, nil, &fixture.otherUser.ID, "Other user's billing preference.")
	assertMemoryToolError(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        otherUserMemoryID.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	}, "another user's memory")

	callMemoryTool(t, ctx, client, "forget_memory", map[string]any{
		"memory_id":        retainedID.String(),
		"_hivy_session_id": fixture.session.ID.String(),
	})
	assertMemoryArchived(t, db, retainedID)
}

func TestMemoryMCPToolsForgetAgentIsolation(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})

	otherAgent := model.Agent{ID: uuid.New(), OrgID: &fixture.org.ID, Name: "Memory MCP Other Agent " + uuid.NewString(), Model: "test", Status: "active"}
	if err := db.Create(&otherAgent).Error; err != nil {
		t.Fatalf("create other agent: %v", err)
	}
	otherToken := &model.Token{
		OrgID: fixture.org.ID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: otherAgent.ID.String(),
		},
	}
	otherClient := connectMemoryToolClient(t, ctx, service, otherToken)

	privateID := seedReadyMemory(t, service, fixture.org.ID, &fixture.agent.ID, nil, "Agent A's private release checklist notes.")
	assertMemoryToolError(t, ctx, otherClient, "forget_memory", map[string]any{
		"memory_id": privateID.String(),
	}, "another agent's memory")
	assertMemoryNotArchived(t, db, privateID)

	sharedID := seedReadyMemory(t, service, fixture.org.ID, nil, nil, "Org-wide memory forgettable by any agent.")
	callMemoryTool(t, ctx, otherClient, "forget_memory", map[string]any{
		"memory_id": sharedID.String(),
	})
	assertMemoryArchived(t, db, sharedID)

	// SQL hardening: an agent-scoped archive must not cross agent boundaries
	// even when the Go guard is bypassed.
	if err := service.Archive(ctx, ArchiveRequest{OrgID: fixture.org.ID, ID: privateID, AgentID: &otherAgent.ID}); !errors.Is(err, gorm.ErrRecordNotFound) {
		t.Fatalf("agent-scoped archive of another agent's memory = %v, want gorm.ErrRecordNotFound", err)
	}
	assertMemoryNotArchived(t, db, privateID)

	// The unscoped path (REST/dashboard) can still archive agent-bound memories.
	if err := service.Archive(ctx, ArchiveRequest{OrgID: fixture.org.ID, ID: privateID}); err != nil {
		t.Fatalf("unscoped archive: %v", err)
	}
	assertMemoryArchived(t, db, privateID)
}

func TestMemoryMCPToolsRetainForOtherAgent(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})
	clientA := connectMemoryToolClient(t, ctx, service, fixture.token)

	agentB := model.Agent{ID: uuid.New(), OrgID: &fixture.org.ID, Name: "Memory MCP Agent B " + uuid.NewString(), Model: "test", Status: "active"}
	archivedAgent := model.Agent{ID: uuid.New(), OrgID: &fixture.org.ID, Name: "Memory MCP Archived " + uuid.NewString(), Model: "test", Status: "archived"}
	otherOrg := model.Org{ID: uuid.New(), Name: "memory-mcp-other-org-" + uuid.NewString(), Active: true, RateLimit: 1000}
	otherOrgAgent := model.Agent{ID: uuid.New(), OrgID: &otherOrg.ID, Name: "Memory MCP Foreign " + uuid.NewString(), Model: "test", Status: "active"}
	for _, row := range []any{&agentB, &archivedAgent, &otherOrg, &otherOrgAgent} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create seed row %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", otherOrg.ID).Delete(&model.Agent{})
		db.Delete(&model.Org{}, "id = ?", otherOrg.ID)
	})
	tokenB := &model.Token{
		OrgID: fixture.org.ID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agentB.ID.String(),
		},
	}
	clientB := connectMemoryToolClient(t, ctx, service, tokenB)

	seeded := callMemoryTool(t, ctx, clientA, "retain_memory", map[string]any{
		"content": "The deploy agent must use service id svc-12345 for production deploys.",
		"target":  map[string]any{"owner": "org", "agent_id": agentB.ID.String()},
	})
	seededMemory := seeded["memory"].(map[string]any)
	seededID := uuid.MustParse(seededMemory["id"].(string))
	var mem model.AgentMemory
	if err := db.First(&mem, "id = ?", seededID).Error; err != nil {
		t.Fatalf("load seeded memory: %v", err)
	}
	if mem.Scope != model.AgentMemoryScopeOrg || mem.AgentID == nil || *mem.AgentID != agentB.ID {
		t.Fatalf("seeded memory target mismatch: %#v", mem)
	}

	rows, err := service.List(ctx, ListRequest{OrgID: fixture.org.ID, AgentID: &agentB.ID, AgentVisibility: AgentVisibilityThisAgent})
	if err != nil {
		t.Fatalf("list agent B memories: %v", err)
	}
	if len(rows) != 1 || rows[0].ID != seededID {
		t.Fatalf("agent B this_agent list = %#v, want only seeded memory", rows)
	}

	ok, err := service.MarkEmbeddingReady(ctx, seededID, 1, testMemoryVector())
	if err != nil || !ok {
		t.Fatalf("mark seeded memory ready: ok=%v err=%v", ok, err)
	}
	searchA := callMemoryTool(t, ctx, clientA, "search_memories", map[string]any{"query": "production deploy service id"})
	if got := len(searchA["results"].([]any)); got != 0 {
		t.Fatalf("agent A search returned %d results, want 0: %#v", got, searchA)
	}
	searchB := callMemoryTool(t, ctx, clientB, "search_memories", map[string]any{"query": "production deploy service id"})
	resultsB := searchB["results"].([]any)
	if len(resultsB) != 1 || resultsB[0].(map[string]any)["id"].(string) != seededID.String() {
		t.Fatalf("agent B search = %#v, want seeded memory", searchB)
	}

	assertMemoryToolError(t, ctx, clientA, "retain_memory", map[string]any{
		"content": "The target cannot carry both a visibility and an agent binding.",
		"target":  map[string]any{"owner": "org", "visibility": "all_agents", "agent_id": agentB.ID.String()},
	}, "cannot include both visibility and agent_id")
	assertMemoryToolError(t, ctx, clientA, "retain_memory", map[string]any{
		"content": "Unknown agents must be rejected before the memory is stored.",
		"target":  map[string]any{"owner": "org", "agent_id": uuid.NewString()},
	}, "must be an active agent in this org")
	assertMemoryToolError(t, ctx, clientA, "retain_memory", map[string]any{
		"content": "Archived agents must be rejected before the memory is stored.",
		"target":  map[string]any{"owner": "org", "agent_id": archivedAgent.ID.String()},
	}, "must be an active agent in this org")
	assertMemoryToolError(t, ctx, clientA, "retain_memory", map[string]any{
		"content": "Agents from other orgs must be rejected before the memory is stored.",
		"target":  map[string]any{"owner": "org", "agent_id": otherOrgAgent.ID.String()},
	}, "must be an active agent in this org")
	assertMemoryToolError(t, ctx, clientA, "retain_memory", map[string]any{
		"content": "User-owned memories cannot be bound to a specific agent.",
		"target":  map[string]any{"owner": "user", "agent_id": agentB.ID.String()},
	}, "target.agent_id requires target.owner org")

	assertMemoryToolError(t, ctx, clientA, "forget_memory", map[string]any{
		"memory_id": seededID.String(),
	}, "another agent's memory")
	assertMemoryNotArchived(t, db, seededID)
	callMemoryTool(t, ctx, clientB, "forget_memory", map[string]any{
		"memory_id": seededID.String(),
	})
	assertMemoryArchived(t, db, seededID)
}

func assertMemoryToolError(t *testing.T, ctx context.Context, client *mcp.ClientSession, name string, args map[string]any, want string) {
	t.Helper()
	result, err := client.CallTool(ctx, &mcp.CallToolParams{
		Name:      name,
		Arguments: args,
	})
	if err != nil {
		t.Fatalf("call %s: %v", name, err)
	}
	if !result.IsError || !strings.Contains(memoryToolText(result), want) {
		t.Fatalf("%s error = %v text %q, want %q", name, result.IsError, memoryToolText(result), want)
	}
}

func assertMemoryArchived(t *testing.T, db *gorm.DB, memoryID uuid.UUID) {
	t.Helper()
	var archived model.AgentMemory
	if err := db.First(&archived, "id = ?", memoryID).Error; err != nil {
		t.Fatalf("load archived memory: %v", err)
	}
	if archived.ArchivedAt == nil {
		t.Fatalf("memory %s was not archived", memoryID)
	}
}

func assertMemoryNotArchived(t *testing.T, db *gorm.DB, memoryID uuid.UUID) {
	t.Helper()
	var mem model.AgentMemory
	if err := db.First(&mem, "id = ?", memoryID).Error; err != nil {
		t.Fatalf("load memory: %v", err)
	}
	if mem.ArchivedAt != nil {
		t.Fatalf("memory %s was archived but should not be", memoryID)
	}
}
