package handler

import (
	"context"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// TestEmployeeEventWriter_AfterWriteNoRace exercises the atomic afterWrite
// publish/consume path concurrently (P2-39). Run with -race; the previous
// plain-func field produced a data race between SetAfterWrite (called after the
// drain goroutine started) and the drain's read.
func TestEmployeeEventWriter_AfterWriteNoRace(t *testing.T) {
	w := &EmployeeEventWriter{}

	if cb := w.loadAfterWrite(); cb != nil {
		t.Fatal("expected nil afterWrite before any Set")
	}

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Writers: repeatedly (re)register callbacks.
	for range 4 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-stop:
					return
				default:
					w.SetAfterWrite(func(context.Context, []model.EmployeeSessionEvent) {})
				}
			}
		}()
	}

	// Readers: repeatedly load and invoke.
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

	// Let them race briefly, then stop all goroutines before asserting
	// anything about the stored value: while the writers are running they can
	// store a non-nil callback at any time, so the nil-clear check below must
	// happen with no concurrent writers active.
	for range 1000 {
		w.SetAfterWrite(func(context.Context, []model.EmployeeSessionEvent) {})
	}
	close(stop)
	wg.Wait()

	// No goroutines are running now: clearing must be observable.
	w.SetAfterWrite(nil)
	if cb := w.loadAfterWrite(); cb != nil {
		t.Fatal("expected nil afterWrite after Set(nil)")
	}
}
