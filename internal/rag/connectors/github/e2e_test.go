package github

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// Drives the full connector surface (CheckpointedConnector +
// SlimConnector) against a 25-PR fixture across 3 pages.
func TestEndToEndIngestion_Through3CScheduler(t *testing.T) {
	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true, IncludeIssues: true,
	}
	c, fp := buildConnector(t, cfg)

	base := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	prsP1 := make([]GithubPR, 10)
	prsP2 := make([]GithubPR, 10)
	prsP3 := make([]GithubPR, 5)
	for i := 0; i < 10; i++ {
		prsP1[i] = makePR(i+1, "open", base.Add(-time.Duration(i)*time.Minute))
	}
	for i := 0; i < 10; i++ {
		prsP2[i] = makePR(i+11, "open", base.Add(-time.Duration(10+i)*time.Minute))
	}
	for i := 0; i < 5; i++ {
		prsP3[i] = makePR(i+21, "open", base.Add(-time.Duration(20+i)*time.Minute))
	}
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, prsP1), 2)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 2, mustMarshal(t, prsP2), 3)
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 3, mustMarshal(t, prsP3), 0)

	// Empty issues page; the orchestrator must still walk into the
	// stage so the checkpoint reaches DONE.
	fp.addPage("GET", "/repos/"+repoFullName+"/issues", 1, []byte(`[]`), 0)

	src := &fixtureSource{cfg: json.RawMessage(`{"repo_owner":"acme","repositories":["widget"]}`)}

	ch, err := c.LoadFromCheckpoint(context.Background(), src, c.DummyCheckpoint(), time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("LoadFromCheckpoint: %v", err)
	}
	docs, fails := drainIngest(t, ch)
	if len(fails) != 0 {
		t.Fatalf("ingest: unexpected failures: %v", fails)
	}
	if len(docs) != 25 {
		t.Fatalf("ingest: expected 25 PR documents, got %d", len(docs))
	}

	slimCh, err := c.ListAllSlim(context.Background(), src)
	if err != nil {
		t.Fatalf("ListAllSlim: %v", err)
	}
	slims, slimFails := drainSlim(t, slimCh)
	if len(slimFails) != 0 {
		t.Fatalf("slim failures: %v", slimFails)
	}
	if len(slims) != 25 {
		t.Fatalf("expected 25 slim docs, got %d", len(slims))
	}
}
