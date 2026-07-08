package memory

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func createManageAgent(t *testing.T, db *gorm.DB, orgID uuid.UUID, name string, isDefault bool) (model.Agent, *model.Token) {
	t.Helper()
	agent := model.Agent{ID: uuid.New(), OrgID: &orgID, Name: name + " " + uuid.NewString(), Model: "test", Status: "active", IsDefault: isDefault}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent %s: %v", name, err)
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

func createManageManager(t *testing.T, db *gorm.DB, orgID uuid.UUID) string {
	t.Helper()
	admin := model.User{ID: uuid.New(), Email: "manage-mgr-" + uuid.NewString() + "@example.com", Name: "Manage Manager"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create manager: %v", err)
	}
	if err := db.Create(&model.OrgMembership{OrgID: orgID, UserID: admin.ID, Role: "admin"}).Error; err != nil {
		t.Fatalf("create manager membership: %v", err)
	}
	t.Cleanup(func() { db.Delete(&model.User{}, "id = ?", admin.ID) })
	return admin.ID.String()
}

func createManageChannel(t *testing.T, db *gorm.DB, orgID, defaultAgentID uuid.UUID, name string) model.Channel {
	t.Helper()
	channel := model.Channel{ID: uuid.New(), OrgID: orgID, Name: name + " " + uuid.NewString(), DefaultAgentID: defaultAgentID, ExposeOrgMemories: true}
	if err := db.Create(&channel).Error; err != nil {
		t.Fatalf("create channel %s: %v", name, err)
	}
	return channel
}

func TestManageMemoriesToolGating(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})

	_, defaultToken := createManageAgent(t, db, fixture.org.ID, "Manage Default", true)

	cases := []struct {
		name  string
		token *model.Token
		want  bool
	}{
		{"default agent", defaultToken, true},
		{"non-default agent", fixture.token, false},
	}
	for _, tc := range cases {
		client := connectMemoryToolClient(t, ctx, service, tc.token)
		tools, err := client.ListTools(ctx, nil)
		if err != nil {
			t.Fatalf("%s: list tools: %v", tc.name, err)
		}
		assertMemoryToolDescriptions(t, tools.Tools)
		got := false
		for _, tool := range tools.Tools {
			if tool.Name == manageMemoriesToolName {
				got = true
			}
		}
		if got != tc.want {
			t.Fatalf("%s: manage_memories registered = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestManageMemoriesSearchAndOverview(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})

	_, defaultToken := createManageAgent(t, db, fixture.org.ID, "Manage Supervisor", true)
	channelA := fixture.channel
	channelB := createManageChannel(t, db, fixture.org.ID, fixture.agent.ID, "manage-channel-b")
	otherOrg := model.Org{ID: uuid.New(), Name: "manage-other-" + uuid.NewString(), Active: true, RateLimit: 1000}
	if err := db.Create(&otherOrg).Error; err != nil {
		t.Fatalf("create other org: %v", err)
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", otherOrg.ID).Delete(&model.AgentMemory{})
		db.Delete(&model.Org{}, "id = ?", otherOrg.ID)
	})

	orgWideID := seedReadyMemoryTagged(t, service, fixture.org.ID, nil, "The org billing policy is Net 30 for annual contracts.", []string{"billing"})
	channelAID := seedReadyMemoryTagged(t, service, fixture.org.ID, &channelA.ID, "Channel A carries the railway service inventory.", []string{"service-discovery"})
	channelBID := seedReadyMemoryTagged(t, service, fixture.org.ID, &channelB.ID, "Channel B deploy target is service api in production.", []string{"service-discovery", "railway"})
	seedReadyMemoryTagged(t, service, otherOrg.ID, nil, "Foreign org memory must never leak.", []string{"billing"})

	client := connectMemoryToolClient(t, ctx, service, defaultToken)
	actorID := createManageManager(t, db, fixture.org.ID)

	// include_facts pins the legacy facts layer; the default observations layer
	// has its own coverage in mcptools_observations_test.go.
	all := callMemoryTool(t, ctx, client, "manage_memories", map[string]any{"action": "search", "query": "service inventory", "include_facts": true, "_hivy_actor_user_id": actorID})
	if all["layer"] != memoryLayerFacts {
		t.Fatalf("include_facts manage search layer = %v, want %q", all["layer"], memoryLayerFacts)
	}
	ids := resultIDSet(all)
	if len(ids) != 3 || !ids[orgWideID.String()] || !ids[channelAID.String()] || !ids[channelBID.String()] {
		t.Fatalf("manage search across all channels = %#v, want 3 spanning both channels + org-wide", all)
	}
	byID := map[string]map[string]any{}
	for _, raw := range all["results"].([]any) {
		item := raw.(map[string]any)
		byID[item["id"].(string)] = item
	}
	if org := byID[orgWideID.String()]; org["org_wide"] != true || org["channel_id"] != nil {
		t.Fatalf("org-wide result mismatch: %#v", org)
	}
	if a := byID[channelAID.String()]; a["org_wide"] != false || a["channel_id"] != channelA.ID.String() || a["channel_name"] != channelA.Name {
		t.Fatalf("channel A result mismatch: %#v", a)
	}

	filtered := callMemoryTool(t, ctx, client, "manage_memories", map[string]any{"action": "search", "query": "deploy target", "channel_id": channelB.ID.String(), "include_facts": true, "_hivy_actor_user_id": actorID})
	fids := resultIDSet(filtered)
	if len(fids) != 1 || !fids[channelBID.String()] {
		t.Fatalf("channel_id filter = %#v, want only channel B's memory", filtered)
	}

	orgOnly := callMemoryTool(t, ctx, client, "manage_memories", map[string]any{"action": "search", "query": "billing policy", "channel_id": "org", "include_facts": true, "_hivy_actor_user_id": actorID})
	oids := resultIDSet(orgOnly)
	if len(oids) != 1 || !oids[orgWideID.String()] {
		t.Fatalf("channel_id=org filter = %#v, want only org-wide memory", orgOnly)
	}

	tagged := callMemoryTool(t, ctx, client, "manage_memories", map[string]any{"action": "search", "query": "railway deploys", "tags": []string{"railway"}, "include_facts": true, "_hivy_actor_user_id": actorID})
	tids := resultIDSet(tagged)
	if len(tids) != 1 || !tids[channelBID.String()] {
		t.Fatalf("tags filter = %#v, want only channel B's memory", tagged)
	}

	assertMemoryToolError(t, ctx, client, "manage_memories", map[string]any{"action": "search", "_hivy_actor_user_id": actorID}, "query is required")
	assertMemoryToolError(t, ctx, client, "manage_memories", map[string]any{"action": "prune", "_hivy_actor_user_id": actorID}, "action must be search or overview")

	// The write actions were removed: memory is read-only to agents.
	assertMemoryToolError(t, ctx, client, "manage_memories", map[string]any{
		"action":              "retain",
		"content":             "Agents may no longer store memories.",
		"_hivy_actor_user_id": actorID,
	}, "action must be search or overview")
	assertMemoryToolError(t, ctx, client, "manage_memories", map[string]any{
		"action":              "forget",
		"memory_id":           orgWideID.String(),
		"_hivy_actor_user_id": actorID,
	}, "action must be search or overview")

	overview := callMemoryTool(t, ctx, client, "manage_memories", map[string]any{"action": "overview", "_hivy_actor_user_id": actorID})
	if overview["total"].(float64) != 3 || overview["org_wide_count"].(float64) != 1 {
		t.Fatalf("overview counts mismatch: %#v", overview)
	}
	channels := overview["channels"].([]any)
	if len(channels) != 2 {
		t.Fatalf("overview channels = %#v, want 2 entries", channels)
	}
	counts := map[string]map[string]any{}
	for _, raw := range channels {
		item := raw.(map[string]any)
		counts[item["channel_id"].(string)] = item
	}
	if a := counts[channelA.ID.String()]; a == nil || a["count"].(float64) != 1 || a["name"] != channelA.Name {
		t.Fatalf("overview channel A entry mismatch: %#v", a)
	}
	topTags := overview["top_tags"].([]any)
	if topTags[0].(map[string]any)["tag"] != "service-discovery" || topTags[0].(map[string]any)["count"].(float64) != 2 {
		t.Fatalf("overview top tag mismatch: %#v", topTags)
	}
}

func TestManageMemoriesActorGate(t *testing.T) {
	ctx := context.Background()
	db := connectMemoryToolTestDB(t)
	fixture := seedMemoryToolFixture(t, db)
	service := NewService(Config{DB: db, Embedder: staticMemoryToolEmbedder{vector: testMemoryVector()}})

	_, defaultToken := createManageAgent(t, db, fixture.org.ID, "Manage Gate Default", true)
	client := connectMemoryToolClient(t, ctx, service, defaultToken)

	// Actor gate: a non-manager human is refused; an admin passes; a no-actor
	// (automated/unattributed) run is refused — the org-wide memory view must
	// not be reachable without a present human org-manager.
	assertMemoryToolError(t, ctx, client, "manage_memories", map[string]any{
		"action":              "overview",
		"_hivy_actor_user_id": fixture.user.ID.String(),
	}, "requires an admin or owner")

	admin := model.User{ID: uuid.New(), Email: "manage-admin-" + uuid.NewString() + "@example.com", Name: "Manage Admin"}
	if err := db.Create(&admin).Error; err != nil {
		t.Fatalf("create admin: %v", err)
	}
	if err := db.Create(&model.OrgMembership{OrgID: fixture.org.ID, UserID: admin.ID, Role: "admin"}).Error; err != nil {
		t.Fatalf("create admin membership: %v", err)
	}
	t.Cleanup(func() { db.Delete(&model.User{}, "id = ?", admin.ID) })
	callMemoryTool(t, ctx, client, "manage_memories", map[string]any{
		"action":              "overview",
		"_hivy_actor_user_id": admin.ID.String(),
	})
	assertMemoryToolError(t, ctx, client, "manage_memories",
		map[string]any{"action": "overview"}, "requires an org admin or owner")
}
