package memory

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func createOrgTestAgent(t *testing.T, fixture memoryToolFixture, name string, isDefault bool, filter *model.ToolFilter) (model.Agent, *model.Token) {
	t.Helper()
	return createOrgTestAgentIn(t, fixture, fixture.org.ID, name, isDefault, filter)
}

func createOrgTestAgentIn(t *testing.T, fixture memoryToolFixture, orgID uuid.UUID, name string, isDefault bool, filter *model.ToolFilter) (model.Agent, *model.Token) {
	t.Helper()
	agent := model.Agent{
		ID:            uuid.New(),
		OrgID:         &orgID,
		Name:          name + " " + uuid.NewString(),
		Model:         "test",
		Status:        "active",
		IsDefault:     isDefault,
		McpToolFilter: filter,
	}
	token := &model.Token{
		OrgID: orgID,
		Meta: model.JSON{
			model.TokenMetaType:    model.TokenTypeAgentProxy,
			model.TokenMetaAgentID: agent.ID.String(),
		},
	}
	return agent, token
}

func seedReadyMemoryTagged(t *testing.T, service *Service, orgID uuid.UUID, agentID, userID *uuid.UUID, content string, tags []string) uuid.UUID {
	t.Helper()
	scope := model.AgentMemoryScopeOrg
	if userID != nil {
		scope = model.AgentMemoryScopeUser
	}
	mem, err := service.Create(context.Background(), CreateRequest{
		OrgID:   orgID,
		AgentID: agentID,
		UserID:  userID,
		Scope:   scope,
		Content: content,
		Tags:    tags,
	})
	if err != nil {
		t.Fatalf("seed tagged memory: %v", err)
	}
	ok, err := service.MarkEmbeddingReady(context.Background(), mem.ID, mem.EmbeddingRevision, testMemoryVector())
	if err != nil || !ok {
		t.Fatalf("mark tagged memory ready: ok=%v err=%v", ok, err)
	}
	return mem.ID
}

func TestOrgMemoriesToolGating(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})

	defaultAgent, defaultToken := createOrgTestAgent(t, fixture, "Org Memories Default", true, nil)
	allowedAgent, allowedToken := createOrgTestAgent(t, fixture, "Org Memories Allowed", false, &model.ToolFilter{Allow: []string{"org_memories"}})
	for _, agent := range []*model.Agent{&defaultAgent, &allowedAgent} {
		if err := db.Create(agent).Error; err != nil {
			t.Fatalf("create agent %s: %v", agent.Name, err)
		}
	}

	cases := []struct {
		name  string
		token *model.Token
		want  bool
	}{
		{"default agent", defaultToken, true},
		{"non-default agent", fixture.token, false},
		{"allow-listed agent", allowedToken, true},
	}
	for _, tc := range cases {
		client := connectMemoryToolClient(t, ctx, service, tc.token)
		tools, err := client.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("%s: list tools: %v", tc.name, err)
		}
		// The three base memory tools must register in every case.
		assertMemoryToolDescriptions(t, tools.Tools)
		got := false
		for _, tool := range tools.Tools {
			if tool.Name == "org_memories" {
				got = true
			}
		}
		if got != tc.want {
			t.Fatalf("%s: org_memories registered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestOrgMemoriesToolSearchAndOverview(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})

	defaultAgent, defaultToken := createOrgTestAgent(t, fixture, "Org Memories Supervisor", true, nil)
	agentB, _ := createOrgTestAgent(t, fixture, "Org Memories Agent B", false, nil)
	otherOrg := model.Org{ID: uuid.New(), Name: "org-memories-other-" + uuid.NewString(), Active: true, RateLimit: 1000}
	for _, row := range []any{&defaultAgent, &agentB, &otherOrg} {
		if err := db.Create(row).Error; err != nil {
			t.Fatalf("create seed row %T: %v", row, err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", otherOrg.ID).Delete(&model.AgentMemory{})
		db.Delete(&model.Org{}, "id = ?", otherOrg.ID)
	})

	sharedID := seedReadyMemoryTagged(t, service, fixture.org.ID, nil, nil, "The org billing policy is Net 30 for annual contracts.", []string{"billing"})
	agentAID := seedReadyMemoryTagged(t, service, fixture.org.ID, &fixture.agent.ID, nil, "Agent A carries the railway service inventory.", []string{"service-discovery"})
	agentBID := seedReadyMemoryTagged(t, service, fixture.org.ID, &agentB.ID, nil, "Agent B deploy target is service api in production.", []string{"service-discovery", "railway"})
	seedReadyMemoryTagged(t, service, fixture.org.ID, nil, &fixture.user.ID, "User prefers weekly summary emails.", nil)
	seedReadyMemoryTagged(t, service, otherOrg.ID, nil, nil, "Foreign org memory must never leak.", []string{"billing"})

	client := connectMemoryToolClient(t, ctx, service, defaultToken)

	search := callMemoryTool(t, ctx, client, "org_memories", map[string]any{
		"action": "search",
		"query":  "service inventory",
	})
	results := search["results"].([]any)
	if len(results) != 3 {
		t.Fatalf("org search returned %d results, want 3 (shared + A + B): %#v", len(results), search)
	}
	byID := map[string]map[string]any{}
	for _, raw := range results {
		item := raw.(map[string]any)
		byID[item["id"].(string)] = item
	}
	shared := byID[sharedID.String()]
	if shared == nil || shared["shared"] != true || shared["agent_id"] != nil || shared["agent_name"] != nil {
		t.Fatalf("shared memory result mismatch: %#v", shared)
	}
	resultA := byID[agentAID.String()]
	if resultA == nil || resultA["shared"] != false || resultA["agent_id"] != fixture.agent.ID.String() || resultA["agent_name"] != fixture.agent.Name {
		t.Fatalf("agent A result mismatch: %#v", resultA)
	}
	resultB := byID[agentBID.String()]
	if resultB == nil || resultB["shared"] != false || resultB["agent_id"] != agentB.ID.String() || resultB["agent_name"] != agentB.Name {
		t.Fatalf("agent B result mismatch: %#v", resultB)
	}

	filtered := callMemoryTool(t, ctx, client, "org_memories", map[string]any{
		"action":   "search",
		"query":    "deploy target",
		"agent_id": agentB.ID.String(),
	})
	filteredResults := filtered["results"].([]any)
	if len(filteredResults) != 1 || filteredResults[0].(map[string]any)["id"].(string) != agentBID.String() {
		t.Fatalf("agent_id filter = %#v, want only agent B's memory", filtered)
	}

	tagged := callMemoryTool(t, ctx, client, "org_memories", map[string]any{
		"action": "search",
		"query":  "railway deploys",
		"tags":   []string{"railway"},
	})
	taggedResults := tagged["results"].([]any)
	if len(taggedResults) != 1 || taggedResults[0].(map[string]any)["id"].(string) != agentBID.String() {
		t.Fatalf("tags filter = %#v, want only agent B's memory", tagged)
	}

	limited := callMemoryTool(t, ctx, client, "org_memories", map[string]any{
		"action": "search",
		"query":  "service inventory",
		"limit":  1,
	})
	if got := len(limited["results"].([]any)); got != 1 {
		t.Fatalf("limit=1 returned %d results, want 1", got)
	}
	clamped := callMemoryTool(t, ctx, client, "org_memories", map[string]any{
		"action": "search",
		"query":  "service inventory",
		"limit":  500,
	})
	if got := len(clamped["results"].([]any)); got != 3 {
		t.Fatalf("limit=500 (clamped) returned %d results, want 3", got)
	}

	assertMemoryToolError(t, ctx, client, "org_memories", map[string]any{
		"action": "search",
	}, "query is required")
	assertMemoryToolError(t, ctx, client, "org_memories", map[string]any{
		"action": "prune",
	}, "action must be search or overview")

	overview := callMemoryTool(t, ctx, client, "org_memories", map[string]any{"action": "overview"})
	if overview["total"].(float64) != 3 || overview["shared_count"].(float64) != 1 || overview["user_scoped_count"].(float64) != 1 {
		t.Fatalf("overview counts mismatch: %#v", overview)
	}
	agents := overview["agents"].([]any)
	if len(agents) != 2 {
		t.Fatalf("overview agents = %#v, want 2 entries", agents)
	}
	agentCounts := map[string]map[string]any{}
	for _, raw := range agents {
		item := raw.(map[string]any)
		agentCounts[item["agent_id"].(string)] = item
	}
	entryA := agentCounts[fixture.agent.ID.String()]
	entryB := agentCounts[agentB.ID.String()]
	if entryA == nil || entryA["count"].(float64) != 1 || entryA["name"] != fixture.agent.Name {
		t.Fatalf("overview agent A entry mismatch: %#v", entryA)
	}
	if entryB == nil || entryB["count"].(float64) != 1 || entryB["name"] != agentB.Name {
		t.Fatalf("overview agent B entry mismatch: %#v", entryB)
	}
	topTags := overview["top_tags"].([]any)
	if len(topTags) != 3 {
		t.Fatalf("overview top_tags = %#v, want 3 entries", topTags)
	}
	first := topTags[0].(map[string]any)
	if first["tag"] != "service-discovery" || first["count"].(float64) != 2 {
		t.Fatalf("overview top tag mismatch: %#v", first)
	}
	tagCounts := map[string]float64{}
	for _, raw := range topTags {
		item := raw.(map[string]any)
		tagCounts[item["tag"].(string)] = item["count"].(float64)
	}
	if tagCounts["billing"] != 1 || tagCounts["railway"] != 1 {
		t.Fatalf("overview tag counts mismatch: %#v", tagCounts)
	}
}

func TestMemoryMCPToolsSupervisorForget(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})

	defaultAgent, defaultToken := createOrgTestAgent(t, fixture, "Supervisor Forget Default", true, nil)
	if err := db.Create(&defaultAgent).Error; err != nil {
		t.Fatalf("create default agent: %v", err)
	}
	supervisor := connectMemoryToolClient(t, ctx, service, defaultToken)
	plain := connectMemoryToolClient(t, ctx, service, fixture.token)

	// Supervisor cannot archive another agent's memory without the flag.
	boundID := seedReadyMemory(t, service, fixture.org.ID, &fixture.agent.ID, nil, "Agent A's private inventory awaiting supervisor cleanup.")
	assertMemoryToolError(t, ctx, supervisor, "forget_memory", map[string]any{
		"memory_id": boundID.String(),
	}, "retry with user_approved true")
	assertMemoryNotArchived(t, db, boundID)

	// With explicit user approval it succeeds.
	callMemoryTool(t, ctx, supervisor, "forget_memory", map[string]any{
		"memory_id":     boundID.String(),
		"user_approved": true,
	})
	assertMemoryArchived(t, db, boundID)

	// The flag does not escalate a non-default agent.
	supervisorBoundID := seedReadyMemory(t, service, fixture.org.ID, &defaultAgent.ID, nil, "Default agent's own private memory.")
	assertMemoryToolError(t, ctx, plain, "forget_memory", map[string]any{
		"memory_id":     supervisorBoundID.String(),
		"user_approved": true,
	}, "only honored for the org's default agent")
	assertMemoryNotArchived(t, db, supervisorBoundID)
	assertMemoryToolError(t, ctx, plain, "forget_memory", map[string]any{
		"memory_id": supervisorBoundID.String(),
	}, "another agent's memory")
	assertMemoryNotArchived(t, db, supervisorBoundID)

	// The user-scope boundary holds even for the supervisor with the flag.
	userMemoryID := seedReadyMemory(t, service, fixture.org.ID, nil, &fixture.user.ID, "User's personal preference stays personal.")
	assertMemoryToolError(t, ctx, supervisor, "forget_memory", map[string]any{
		"memory_id":     userMemoryID.String(),
		"user_approved": true,
	}, "another user's memory")
	assertMemoryNotArchived(t, db, userMemoryID)

	// Own and shared memories still work without any flag.
	callMemoryTool(t, ctx, supervisor, "forget_memory", map[string]any{
		"memory_id": supervisorBoundID.String(),
	})
	assertMemoryArchived(t, db, supervisorBoundID)
	sharedID := seedReadyMemory(t, service, fixture.org.ID, nil, nil, "Shared org memory forgettable by the supervisor too.")
	callMemoryTool(t, ctx, supervisor, "forget_memory", map[string]any{
		"memory_id": sharedID.String(),
	})
	assertMemoryArchived(t, db, sharedID)
}
