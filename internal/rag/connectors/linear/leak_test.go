package linear

import (
	"context"
	"fmt"
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

// The ListAllSlim producer goroutine must unwind (not leak) when the
// consumer reads one item, stops, and cancels the context.
func TestSlim_ProducerExitsOnConsumerAbandon(t *testing.T) {
	baseline := runtime.NumGoroutine()

	fp := newFakeProxy()
	nodes := make([]string, 50)
	for i := range nodes {
		nodes[i] = projectNode(fmt.Sprintf("P%d", i))
	}
	fp.stub("TeamProjects", 200, projectsPage("", false, nodes...))
	fp.stub("SlimTeamIssues", 200, slimIssuesPage("", false))

	c := NewConnector(Config{TeamIDs: []string{"T1"}}, fp)
	// Tiny buffer so the producer blocks quickly once the consumer stops.
	c.channelBuf = 1

	ctx, cancel := context.WithCancel(context.Background())
	ch, err := c.ListAllSlim(ctx, newFixtureSource("{}"))
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
