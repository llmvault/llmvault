package github

import (
	"context"
	"encoding/json"
	"runtime"
	"testing"
	"time"
)

// waitGoroutinesBelow polls until the live goroutine count drops to at
// most baseline+slack, or fails the test on timeout. A leaked producer
// blocked forever on a full output channel keeps the count elevated.
func waitGoroutinesBelow(t *testing.T, baseline, slack int) {
	t.Helper()
	deadline := time.After(3 * time.Second)
	for {
		runtime.GC()
		if runtime.NumGoroutine() <= baseline+slack {
			return
		}
		select {
		case <-deadline:
			t.Fatalf("producer goroutine leaked: goroutines=%d baseline=%d", runtime.NumGoroutine(), baseline)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// The ListAllSlim producer goroutine must unwind (not leak) when the consumer
// reads one item, stops, and cancels the context.
func TestSlim_ProducerExitsOnConsumerAbandon(t *testing.T) {
	baseline := runtime.NumGoroutine()

	cfg := GithubConfig{
		RepoOwner: "acme", Repositories: []string{"widget"},
		StateFilter: "all", IncludePRs: true, IncludeIssues: false,
	}
	c, fp := buildConnector(t, cfg)
	// Tiny buffer so the producer blocks quickly once the consumer stops.
	c.channelBuf = 1

	base := time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	prs := make([]GithubPR, 50)
	for i := range prs {
		prs[i] = makePR(i+1, "open", base.Add(-time.Duration(i)*time.Minute))
	}
	fp.addPage("GET", "/repos/"+repoFullName+"/pulls", 1, mustMarshal(t, prs), 0)

	src := &fixtureSource{cfg: json.RawMessage(`{"repo_owner":"acme","repositories":["widget"]}`)}
	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.ListAllSlim(ctx, src)
	if err != nil {
		t.Fatalf("ListAllSlim: %v", err)
	}

	// Consume one item, then abandon: cancel and never read again.
	select {
	case <-ch:
	case <-time.After(2 * time.Second):
		t.Fatal("no first item produced")
	}
	cancel()

	waitGoroutinesBelow(t, baseline, 0)
}
