package handler

import (
	"context"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// The atomic afterWrite publish/consume path must be race-free (run with -race):
// the plain-func field used to race SetAfterWrite against the drain's read.
func TestAgentEventWriter_AfterWriteNoRace(t *testing.T) {
	w := &AgentEventWriter{}

	if cb := w.loadAfterWrite(); cb != nil {
		t.Fatal("expected nil afterWrite before any Set")
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					w.SetAfterWrite(func(context.Context, []model.AgentSessionEvent) {})
				}
			}
		}()
	}

	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					if cb := w.loadAfterWrite(); cb != nil {
						cb(context.Background(), nil)
					}
				}
			}
		}()
	}

	// Stop all goroutines before asserting: the nil-clear check below must run
	// with no concurrent writers active.
	for range 1000 {
		w.SetAfterWrite(func(context.Context, []model.AgentSessionEvent) {})
	}
	close(stop)
	wg.Wait()

	w.SetAfterWrite(nil)
	if cb := w.loadAfterWrite(); cb != nil {
		t.Fatal("expected nil afterWrite after Set(nil)")
	}
}
