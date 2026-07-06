package linear

import (
	"context"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func slimIssuesPage(cursor string, hasNext bool, ids ...string) string {
	nodes := make([]string, 0, len(ids))
	for _, id := range ids {
		nodes = append(nodes, `{"id":"`+id+`"}`)
	}
	return `{"data":{"issues":{"nodes":[` + join(nodes) + `],"pageInfo":` +
		pageInfoJSON(cursor, hasNext) + `}}}`
}

func drainSlim(t *testing.T, ch <-chan interfaces.SlimDocOrFailure) (docIDs []string, fails []*interfaces.ConnectorFailure) {
	t.Helper()
	for item := range ch {
		if item.Failure != nil {
			fails = append(fails, item.Failure)
			continue
		}
		docIDs = append(docIDs, item.Slim.DocID)
	}
	return docIDs, fails
}

// The slim doc-id alphabet MUST equal the ingest alphabet for identical
// source state, or the prune loop would delete live documents.
func TestSlimSetEqualsIngestSet(t *testing.T) {
	// Ingest walk.
	ingestFP := newFakeProxy()
	ingestFP.stub("TeamProjects", 200, projectsPage("", false, projectNode("P1", "I1")))
	ingestFP.stub("TeamProjects", 200, projectsPage("", false, projectNode("P2", "I1")))
	ingestFP.stub("TeamIssues", 200, issuesPage("", false,
		issueNode("1", false, ""), issueNode("2", false, ""), issueNode("3", false, "")))
	ingestC := NewConnector(Config{TeamIDs: []string{"T1", "T2"}}, ingestFP)

	ich, err := ingestC.LoadFromCheckpoint(context.Background(), newFixtureSource("{}"), ingestC.DummyCheckpoint(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("ingest LoadFromCheckpoint: %v", err)
	}
	ingestIDs, ifails := drainIngest(t, ich)
	if len(ifails) != 0 {
		t.Fatalf("ingest failures: %+v", ifails)
	}

	// Slim walk over the same logical source state.
	slimFP := newFakeProxy()
	slimFP.stub("TeamProjects", 200, projectsPage("", false, projectNode("P1", "I1")))
	slimFP.stub("TeamProjects", 200, projectsPage("", false, projectNode("P2", "I1")))
	slimFP.stub("SlimTeamIssues", 200, slimIssuesPage("", false, "1", "2", "3"))
	slimC := NewConnector(Config{TeamIDs: []string{"T1", "T2"}}, slimFP)

	sch, err := slimC.ListAllSlim(context.Background(), newFixtureSource("{}"))
	if err != nil {
		t.Fatalf("ListAllSlim: %v", err)
	}
	slimIDs, sfails := drainSlim(t, sch)
	if len(sfails) != 0 {
		t.Fatalf("slim failures: %+v", sfails)
	}

	if a, b := sortedCopy(ingestIDs), sortedCopy(slimIDs); !equalStrings(a, b) {
		t.Fatalf("slim set != ingest set:\n ingest=%v\n slim  =%v", a, b)
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
