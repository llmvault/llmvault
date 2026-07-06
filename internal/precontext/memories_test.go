package precontext

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

// fakeRecallSource scripts every recall layer so tests can drive the
// directives-first ordering, the digest passthrough, and the fallback chain.
type fakeRecallSource struct {
	directives      []model.AgentDirective
	directivesErr   error
	digest          string
	digestErr       error
	observations    []model.AgentObservation
	observationsErr error

	directiveScopes []memory.ChannelScope
	obsScopes       []memory.ChannelScope
	obsLimits       []int
	digestCalls     int
}

func (f *fakeRecallSource) ActiveDirectives(_ context.Context, _ uuid.UUID, scope memory.ChannelScope) ([]model.AgentDirective, error) {
	f.directiveScopes = append(f.directiveScopes, scope)
	return f.directives, f.directivesErr
}

func (f *fakeRecallSource) ChannelMemoryDigest(context.Context, uuid.UUID, uuid.UUID) (string, error) {
	f.digestCalls++
	return f.digest, f.digestErr
}

func (f *fakeRecallSource) TopObservations(_ context.Context, _ uuid.UUID, scope memory.ChannelScope, limit int) ([]model.AgentObservation, error) {
	f.obsScopes = append(f.obsScopes, scope)
	f.obsLimits = append(f.obsLimits, limit)
	return f.observations, f.observationsErr
}

func recallRequest() Request {
	return Request{OrgID: uuid.New(), ChannelID: uuid.New(), IncludeOrgMemories: true}
}

func fetchRecall(t *testing.T, source *fakeRecallSource, req Request) string {
	t.Helper()
	service := NewService(Config{Memories: source})
	out, err := service.fetchMemoriesSection(context.Background(), req)
	if err != nil {
		t.Fatalf("fetchMemoriesSection returned error: %v", err)
	}
	return out
}

func TestMemoriesSectionRendersDirectivesThenDigest(t *testing.T) {
	source := &fakeRecallSource{
		directives: []model.AgentDirective{
			{Content: "Always deploy through Railway."},
			{Content: "Never email customers before 9am."},
		},
		digest: "- [decision] Chose Railway over Fly in June 2026.\n- [convention] PRs need one approval.",
	}
	out := fetchRecall(t, source, recallRequest())

	rulesIdx := strings.Index(out, "### Rules")
	digestIdx := strings.Index(out, "- [decision] Chose Railway over Fly in June 2026.")
	if !strings.HasPrefix(out, "## Memories") || rulesIdx < 0 || digestIdx < 0 {
		t.Fatalf("memories section missing parts: %q", out)
	}
	if rulesIdx > digestIdx {
		t.Fatalf("directives must render before the digest: %q", out)
	}
	for _, rule := range []string{"- Always deploy through Railway.", "- Never email customers before 9am."} {
		if !strings.Contains(out, rule) {
			t.Fatalf("directive bullet %q missing: %q", rule, out)
		}
	}
	if !strings.Contains(out, "- [convention] PRs need one approval.") {
		t.Fatalf("digest content must pass through as-is: %q", out)
	}
	if len(source.obsScopes) != 0 {
		t.Fatalf("digest hit must not query observations (%d)", len(source.obsScopes))
	}
}

func TestMemoriesSectionFallsBackToObservations(t *testing.T) {
	channelID := uuid.New()
	source := &fakeRecallSource{
		digest: "   ",
		observations: []model.AgentObservation{
			{ChannelID: &channelID, Kind: "decision", Content: "Chose Postgres over Mongo for the ledger.", ProofCount: 4},
			{Kind: "org-fact", Content: "ACME is the largest customer."},
		},
	}
	out := fetchRecall(t, source, recallRequest())

	if !strings.Contains(out, "- [decision] Chose Postgres over Mongo for the ledger.") ||
		!strings.Contains(out, "- [org-fact] ACME is the largest customer.") {
		t.Fatalf("observation fallback lines missing: %q", out)
	}
	if len(source.obsLimits) != 1 || source.obsLimits[0] != fallbackObservationLimit {
		t.Fatalf("observation fallback limits = %v, want [%d]", source.obsLimits, fallbackObservationLimit)
	}
}

func TestMemoriesSectionDegradesToObservationsWhenDigestFails(t *testing.T) {
	source := &fakeRecallSource{
		digestErr: errors.New("digest read failed"),
		observations: []model.AgentObservation{
			{Kind: "finding", Content: "Survivor fact."},
		},
	}
	out := fetchRecall(t, source, recallRequest())
	if !strings.Contains(out, "- [finding] Survivor fact.") {
		t.Fatalf("digest failure must degrade to observations: %q", out)
	}
	if source.digestCalls != 1 || len(source.obsScopes) != 1 {
		t.Fatalf("fallback chain calls digest=%d obs=%d, want 1/1", source.digestCalls, len(source.obsScopes))
	}
}

func TestMemoriesSectionRendersRulesWhenRecallFails(t *testing.T) {
	source := &fakeRecallSource{
		directives:      []model.AgentDirective{{Content: "Always require human approval for refunds."}},
		digestErr:       errors.New("digest read failed"),
		observationsErr: errors.New("observations read failed"),
	}
	out := fetchRecall(t, source, recallRequest())
	if !strings.Contains(out, "### Rules") || !strings.Contains(out, "human approval for refunds") {
		t.Fatalf("directives must survive a recall failure: %q", out)
	}
	if strings.Contains(out, memoryKnowledgeHeading) {
		t.Fatalf("failed recall must not render a knowledge subsection: %q", out)
	}

	// Without directives the same failures degrade to an omitted section, not
	// an error: legacy agent_memories are never injected as a fallback.
	source = &fakeRecallSource{
		digestErr:       errors.New("digest read failed"),
		observationsErr: errors.New("observations read failed"),
	}
	out = fetchRecall(t, source, recallRequest())
	if out != "" {
		t.Fatalf("expected empty section when every layer fails, got %q", out)
	}
}

func TestMemoriesSectionScopeFollowsExposeOrgMemories(t *testing.T) {
	for _, include := range []bool{true, false} {
		source := &fakeRecallSource{}
		req := recallRequest()
		req.IncludeOrgMemories = include
		fetchRecall(t, source, req)
		if len(source.directiveScopes) != 1 || len(source.obsScopes) != 1 {
			t.Fatalf("include=%v: scope calls directives=%d obs=%d", include, len(source.directiveScopes), len(source.obsScopes))
		}
		for name, scope := range map[string]memory.ChannelScope{
			"directives":   source.directiveScopes[0],
			"observations": source.obsScopes[0],
		} {
			if scope.ChannelID == nil || *scope.ChannelID != req.ChannelID || scope.IncludeOrgMemories != include || scope.AllChannels {
				t.Fatalf("include=%v: %s scope = %#v", include, name, scope)
			}
		}
	}
}

func TestMemoriesSectionBudgetPrefersDirectives(t *testing.T) {
	source := &fakeRecallSource{
		directives: []model.AgentDirective{
			{Content: "RULE-ONE always applies."},
			{Content: "RULE-TWO always applies."},
		},
		digest: "- " + strings.Repeat("x", MemoriesBudgetBytes*2),
	}
	out := fetchRecall(t, source, recallRequest())
	if len(out) > MemoriesBudgetBytes {
		t.Fatalf("memories section exceeded budget: %d > %d", len(out), MemoriesBudgetBytes)
	}
	if !strings.Contains(out, "RULE-ONE always applies.") || !strings.Contains(out, "RULE-TWO always applies.") {
		t.Fatalf("directives were trimmed before the digest: %q", out[:200])
	}
	if !strings.HasSuffix(out, "...") {
		t.Fatalf("oversized digest should be the trimmed part: %q", out[len(out)-40:])
	}
	rulesIdx := strings.Index(out, "### Rules")
	digestIdx := strings.Index(out, "- xxx")
	if rulesIdx < 0 || digestIdx < 0 || rulesIdx > digestIdx {
		t.Fatalf("ordering lost under budget pressure (rules=%d digest=%d)", rulesIdx, digestIdx)
	}
}

func TestMemoriesSectionEmptyWhenNoLayersHaveContent(t *testing.T) {
	out := fetchRecall(t, &fakeRecallSource{}, recallRequest())
	if out != "" {
		t.Fatalf("expected empty section, got %q", out)
	}
}
