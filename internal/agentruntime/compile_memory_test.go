package agentruntime

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/model"
)

func TestListPreloadMemoriesFetchesTaggedQueries(t *testing.T) {
	fake := &fakeMemoryRecall{listResponse: &hindsight.ListMemoriesResponse{
		Items: []map[string]any{{
			"content":     "The Platform team requires integration tests for agent-runtime changes.",
			"document_id": "doc-1",
			"memory_type": "technical_context",
			"tags":        []any{"scope:provider", "provider:github-app"},
		}},
	}}
	queries := []hindsight.MemoryListQuery{
		{
			Name: "github-app",
			TagGroups: []any{map[string]any{
				"tags":  []string{"scope:provider", "provider:github-app"},
				"match": "all_strict",
			}},
		},
		{Name: "org", ExcludeTags: []string{"scope:provider", "scope:resource"}},
	}

	results := listPreloadMemories(context.Background(), fake, "org-bank", queries)

	if len(results) != 2 {
		t.Fatalf("results len = %d, want one response per query", len(results))
	}
	if len(fake.listRequests) != 2 {
		t.Fatalf("list requests = %#v", fake.listRequests)
	}
	var providerRequest, orgRequest *hindsight.ListMemoriesOptions
	for i := range fake.listRequests {
		req := &fake.listRequests[i]
		if len(req.TagGroups) > 0 {
			providerRequest = req
		}
		if containsString(req.ExcludeTags, "scope:resource") {
			orgRequest = req
		}
	}
	if providerRequest == nil || orgRequest == nil {
		t.Fatalf("missing provider or org request: %#v", fake.listRequests)
	}
	if providerRequest.Limit != memoryPreloadPerQueryLimit || orgRequest.Limit != memoryPreloadPerQueryLimit {
		t.Fatalf("unexpected limits: provider=%d org=%d", providerRequest.Limit, orgRequest.Limit)
	}
}

func TestListPreloadMemoriesToleratesListFailures(t *testing.T) {
	fake := &fakeMemoryRecall{err: errors.New("offline")}

	results := listPreloadMemories(context.Background(), fake, "org-bank", []hindsight.MemoryListQuery{{Name: "org"}})

	if len(results) != 0 {
		t.Fatalf("results = %#v", results)
	}
}

func TestCompile_DoesNotPreloadMemoryWithoutValidatedScopes(t *testing.T) {
	orgID := uuid.New()
	agent := model.Agent{
		ID:    uuid.New(),
		OrgID: &orgID,
		Name:  "Aria",
		Model: DefaultAgentModel,
	}
	fake := &fakeMemoryRecall{listResponse: &hindsight.ListMemoriesResponse{}}

	def, err := Compile(context.Background(), CompileDeps{Hindsight: fake, Cfg: &config.Config{}}, &agent)
	if err != nil {
		t.Fatalf("compile: %v", err)
	}
	if len(fake.listRequests) != 0 {
		t.Fatalf("expected no broad memory list without validated scopes, got %#v", fake.listRequests)
	}
	memory, ok := def.Context["memory"].(MemoryContext)
	if !ok {
		t.Fatalf("memory context missing or wrong type: %#v", def.Context["memory"])
	}
	if len(memory.Entries) != 0 {
		t.Fatalf("expected empty memory entries, got %#v", memory.Entries)
	}
}
