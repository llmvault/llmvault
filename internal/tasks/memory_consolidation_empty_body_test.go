package tasks

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

// emptyBodyConsolidationEmbedder is the seam used to verify the empty-body
// error from the embedder propagates through the consolidation handler.
// Issue #214 explicitly calls for a test at this boundary: the handler
// wraps the embedder error with `embed facts for consolidation: %w`, and
// the resulting error must (a) keep the original informative message so
// operators can diagnose it and (b) be returned to asynq so the task
// retries instead of being archived as a permanent failure.
type emptyBodyConsolidationEmbedder struct{}

func (emptyBodyConsolidationEmbedder) Embed(_ context.Context, _ []string) ([][]float32, int, error) {
	return nil, 0, &emptyBodyError{status: http.StatusOK}
}

type emptyBodyError struct{ status int }

func (e *emptyBodyError) Error() string {
	return "embed: empty response body (status=" + itoa(e.status) + ")"
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	buf := [20]byte{}
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	if negative {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}

// TestConsolidationHandlerRetriesOnEmptyBodyEmbed asserts the handler
// returns an error that (1) keeps the embedder's `empty response body`
// message and (2) is wrapped with `embed facts for consolidation: %w`
// so the stack matches the production code path in
// memory_consolidation.go:144. asynq treats any non-nil return as
// retryable up to MaxRetry(3), so this also covers the retryability
// requirement from issue #214.
func TestConsolidationHandlerRetriesOnEmptyBodyEmbed(t *testing.T) {
	ctx := context.Background()
	db := connectTestDB(t)

	org := model.Org{ID: uuid.New(), Name: "consolidate-empty-" + uuid.NewString(), Active: true, RateLimit: 1000}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "consolidate-empty-team-" + uuid.NewString()}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Consolidate Empty " + uuid.NewString(), Model: "test", Status: "active"}
	channel := model.Channel{
		ID: uuid.New(), OrgID: org.ID, TeamID: team.ID,
		Name: "consolidate-empty-" + uuid.NewString(), Category: "general",
		DefaultAgentID: agent.ID,
	}
	for _, seed := range []error{
		db.Create(&org).Error, db.Create(&team).Error,
		db.Create(&agent).Error, db.Create(&channel).Error,
	} {
		if seed != nil {
			t.Fatalf("seed fixtures: %v", seed)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.AgentMemory{})
		db.Where("org_id = ?", org.ID).Delete(&model.Channel{})
		db.Where("org_id = ?", org.ID).Delete(&model.Agent{})
		db.Where("org_id = ?", org.ID).Delete(&model.Team{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	fact := model.AgentMemory{
		OrgID:     org.ID,
		ChannelID: &channel.ID,
		Content:   "ACME prefers invoices in EUR.",
		Metadata:  model.JSON{"source": "mcp_memory_tool", "agent_id": agent.ID.String()},
	}
	if err := db.Omit("Org", "Channel").Create(&fact).Error; err != nil {
		t.Fatalf("seed fact: %v", err)
	}

	svc := memory.NewService(memory.Config{DB: db, Embedder: emptyBodyConsolidationEmbedder{}})
	handler := &MemoryConsolidationHandler{
		db:        db,
		memorySvc: svc,
		memoryCfg: MemoryEmbeddingConfig{Model: memory.DefaultEmbeddingModel, Dim: memory.DefaultEmbeddingDim},
		complete: func(_ context.Context, _, _ string, _ string, _ json.RawMessage, _ int) (string, error) {
			t.Fatal("LLM completion must not be reached when embed fails")
			return "", nil
		},
	}

	task, _, err := NewMemoryConsolidateTask(MemoryConsolidatePayload{OrgID: org.ID, ChannelID: channel.ID})
	if err != nil {
		t.Fatalf("build task: %v", err)
	}

	err = handler.Handle(ctx, task)
	if err == nil {
		t.Fatal("expected handler to return an error when embedder returns empty body, got nil")
	}
	if !strings.Contains(err.Error(), "embed facts for consolidation") {
		t.Fatalf("handler error must be wrapped with the consolidation prefix, got: %v", err)
	}
	if !strings.Contains(err.Error(), "embed: empty response body") {
		t.Fatalf("handler error must surface the embedder message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "status=200") {
		t.Fatalf("handler error must include status=200, got: %v", err)
	}
	// asynq treats any non-nil error as retryable; verify the task is
	// not somehow swallowed by a marker.
	if isAsynqSkip(err) {
		t.Fatalf("handler must not return asynq.SkipRetry: %v", err)
	}
}

func isAsynqSkip(err error) bool {
	return err == asynq.SkipRetry
}
