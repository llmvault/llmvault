package linear

import (
	"context"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// --- fixture-envelope builders -------------------------------------------
//
// These assemble the GraphQL response JSON the client parser consumes.
// The envelope shapes (data.team.projects, data.issues, data.issue.comments)
// mirror the fixtures_test.go contract; if client.go's parser expects a
// different nesting, edit only these helpers.

func projectNode(id string, initiativeIDs ...string) string {
	inits := make([]string, 0, len(initiativeIDs))
	for _, iid := range initiativeIDs {
		inits = append(inits, `{"id":"`+iid+`","name":"Init `+iid+`","url":"https://linear.app/i/`+iid+`","description":"d","content":"c","updatedAt":"2026-01-01T00:00:00Z"}`)
	}
	return `{"id":"` + id + `","name":"Project ` + id + `","url":"https://linear.app/p/` + id +
		`","description":"pd","content":"pc","updatedAt":"2026-01-01T00:00:00Z",` +
		`"initiatives":{"nodes":[` + join(inits) + `]}}`
}

func projectsPage(cursor string, hasNext bool, nodes ...string) string {
	return `{"data":{"team":{"projects":{"nodes":[` + join(nodes) + `],"pageInfo":` +
		pageInfoJSON(cursor, hasNext) + `}}}}`
}

func issueNode(id string, commentsHasNext bool, commentsCursor string) string {
	return `{"id":"` + id + `","identifier":"ENG-` + id + `","title":"Issue ` + id +
		`","description":"desc","url":"https://linear.app/issue/` + id +
		`","updatedAt":"2026-01-01T00:00:00Z","comments":{"nodes":[` +
		`{"id":"c0-` + id + `","body":"inline","url":""}` +
		`],"pageInfo":` + pageInfoJSON(commentsCursor, commentsHasNext) + `}}`
}

func issuesPage(cursor string, hasNext bool, nodes ...string) string {
	return `{"data":{"issues":{"nodes":[` + join(nodes) + `],"pageInfo":` +
		pageInfoJSON(cursor, hasNext) + `}}}`
}

func commentsPage(cursor string, hasNext bool) string {
	return `{"data":{"issue":{"comments":{"nodes":[{"id":"c1","body":"more","url":""}],"pageInfo":` +
		pageInfoJSON(cursor, hasNext) + `}}}}`
}

func pageInfoJSON(cursor string, hasNext bool) string {
	if hasNext {
		return `{"hasNextPage":true,"endCursor":"` + cursor + `"}`
	}
	return `{"hasNextPage":false,"endCursor":""}`
}

func join(parts []string) string {
	out := ""
	for i, p := range parts {
		if i > 0 {
			out += ","
		}
		out += p
	}
	return out
}

// --- drain helpers -------------------------------------------------------

func drainIngest(t *testing.T, ch <-chan interfaces.DocumentOrFailure) (docIDs []string, fails []*interfaces.ConnectorFailure) {
	t.Helper()
	for item := range ch {
		if item.Failure != nil {
			fails = append(fails, item.Failure)
			continue
		}
		docIDs = append(docIDs, item.Doc.DocID)
	}
	return docIDs, fails
}

func finalStage(t *testing.T, c *LinearConnector) Stage {
	t.Helper()
	raw, err := c.FinalCheckpoint()
	if err != nil {
		t.Fatalf("FinalCheckpoint: %v", err)
	}
	cp, err := unmarshalCheckpoint(raw)
	if err != nil {
		t.Fatalf("unmarshalCheckpoint: %v", err)
	}
	return cp.Stage
}

func sortedCopy(in []string) []string {
	out := append([]string(nil), in...)
	sort.Strings(out)
	return out
}

// --- tests ---------------------------------------------------------------

// Full walk over two teams that share one initiative: the initiative is
// emitted once. Three issues, one with a paginated comment page.
func TestFullWalk_TwoTeamsSharedInitiative(t *testing.T) {
	fp := newFakeProxy()
	// FIFO per op: first TeamProjects call is T1, second is T2.
	fp.stub("TeamProjects", 200, projectsPage("", false, projectNode("P1", "I1")))
	fp.stub("TeamProjects", 200, projectsPage("", false, projectNode("P2", "I1")))
	fp.stub("TeamIssues", 200, issuesPage("", false,
		issueNode("1", false, ""),
		issueNode("2", true, "cc"),
		issueNode("3", false, ""),
	))
	fp.stub("IssueComments", 200, commentsPage("", false))

	c := NewConnector(Config{TeamIDs: []string{"T1", "T2"}}, fp)
	ch, err := c.LoadFromCheckpoint(context.Background(), newFixtureSource("{}"), c.DummyCheckpoint(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	docIDs, fails := drainIngest(t, ch)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}

	want := sortedCopy([]string{
		docIDForProject("P1"), docIDForProject("P2"),
		docIDForInitiative("I1"),
		docIDForIssue("1"), docIDForIssue("2"), docIDForIssue("3"),
	})
	got := sortedCopy(docIDs)
	if len(got) != len(want) {
		t.Fatalf("doc count: got %d %v, want %d %v", len(got), got, len(want), want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("doc set mismatch: got %v want %v", got, want)
		}
	}
	if s := finalStage(t, c); s != StageDone {
		t.Fatalf("final stage = %q, want DONE", s)
	}
}

// A source with no configured teams ingests nothing and lands on DONE.
func TestNoScope_ZeroDocs(t *testing.T) {
	fp := newFakeProxy()
	c := NewConnector(Config{TeamIDs: nil}, fp)
	ch, err := c.LoadFromCheckpoint(context.Background(), newFixtureSource("{}"), c.DummyCheckpoint(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	docIDs, fails := drainIngest(t, ch)
	if len(docIDs) != 0 || len(fails) != 0 {
		t.Fatalf("expected zero docs/failures, got docs=%v fails=%v", docIDs, fails)
	}
	if s := finalStage(t, c); s != StageDone {
		t.Fatalf("final stage = %q, want DONE", s)
	}
}

// A team whose projects page errors emits one entity failure and is
// dropped; the other team still processes and the run completes.
func TestTeamPageError_OtherTeamStillProcessed(t *testing.T) {
	fp := newFakeProxy()
	// GraphQL-level error (status 200) is permanent — no retry — so the
	// FIFO stub queue stays aligned: T1 fails, T2 succeeds.
	fp.stub("TeamProjects", 200, `{"errors":[{"message":"boom"}]}`) // T1 fails
	fp.stub("TeamProjects", 200, projectsPage("", false, projectNode("P2")))
	fp.stub("TeamIssues", 200, issuesPage("", false))

	c := NewConnector(Config{TeamIDs: []string{"T1", "T2"}}, fp)
	ch, err := c.LoadFromCheckpoint(context.Background(), newFixtureSource("{}"), c.DummyCheckpoint(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	docIDs, fails := drainIngest(t, ch)
	if len(fails) != 1 {
		t.Fatalf("expected exactly 1 failure, got %d: %+v", len(fails), fails)
	}
	if fails[0].FailedEntity == nil || fails[0].FailedEntity.EntityID != "T1" {
		t.Fatalf("expected entity failure for T1, got %+v", fails[0])
	}
	found := false
	for _, id := range docIDs {
		if id == docIDForProject("P2") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected P2 project doc, got %v", docIDs)
	}
	if s := finalStage(t, c); s != StageDone {
		t.Fatalf("final stage = %q, want DONE", s)
	}
}

// updatedSince is derived from a non-zero start (minus pollOverlap) and
// propagated into the issues query variables.
func TestUpdatedSincePropagation(t *testing.T) {
	fp := newFakeProxy()
	fp.stub("TeamProjects", 200, projectsPage("", false))
	fp.stub("TeamIssues", 200, issuesPage("", false))

	c := NewConnector(Config{TeamIDs: []string{"T1"}}, fp)
	start := time.Date(2026, 5, 1, 12, 0, 0, 0, time.UTC)
	ch, err := c.LoadFromCheckpoint(context.Background(), newFixtureSource("{}"), c.DummyCheckpoint(), start, time.Time{})
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	if _, fails := drainIngest(t, ch); len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}

	if !anyRequestContains(fp, "TeamIssues", "updatedAt") {
		t.Fatalf("expected an issues request carrying an updatedAt filter; requests=%v", fp.requests)
	}
}

// anyRequestContains reports whether any logged request body for op
// mentions needle. fakeProxy.requests holds the raw request bodies.
func anyRequestContains(fp *fakeProxy, op, needle string) bool {
	for _, raw := range fp.requests {
		body := string(raw)
		if strings.Contains(body, op) && strings.Contains(body, needle) {
			return true
		}
	}
	return false
}
