package linear

import (
	"context"
	"strings"
	"testing"
	"time"
)

// Resuming from a checkpoint parked mid-ISSUES must skip the PROJECTS
// stage entirely and continue the issue walk from the persisted cursor.
func TestResumeMidIssues_SkipsProjectsAndUsesCursor(t *testing.T) {
	fp := newFakeProxy()
	// Only issues should be fetched. If PROJECTS ran, this TeamProjects
	// stub would be consumed — we assert below that it never is.
	fp.stub("TeamProjects", 500, `{"errors":[{"message":"should not be called"}]}`)
	fp.stub("TeamIssues", 200, issuesPage("", false, issueNode("9", false, "")))

	c := NewConnector(Config{TeamIDs: []string{"T1"}}, fp)
	cp := LinearCheckpoint{Stage: StageIssues, IssuesCursor: "resume-cursor"}

	ch, err := c.LoadFromCheckpoint(context.Background(), newFixtureSource("{}"), cp, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	docIDs, fails := drainIngest(t, ch)
	if len(fails) != 0 {
		t.Fatalf("unexpected failures: %+v", fails)
	}
	if len(docIDs) != 1 || docIDs[0] != docIDForIssue("9") {
		t.Fatalf("expected only linear_issue_9, got %v", docIDs)
	}

	// No projects request may have been issued.
	for _, raw := range fp.requests {
		if strings.Contains(string(raw), "TeamProjects") {
			t.Fatalf("PROJECTS stage ran on resume; request=%s", string(raw))
		}
	}
	// The persisted cursor must have seeded the first issues fetch.
	if !anyRequestContains(fp, "TeamIssues", "resume-cursor") {
		t.Fatalf("resume cursor not propagated into issues query; requests=%v", fp.requests)
	}
	if s := finalStage(t, c); s != StageDone {
		t.Fatalf("final stage = %q, want DONE", s)
	}
}
