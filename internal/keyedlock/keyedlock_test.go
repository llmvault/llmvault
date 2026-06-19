package keyedlock

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sourcegraph/conc/pool"

	"github.com/usehivy/hivy/internal/goroutine"
)

func TestLockerSerializesSameKey(t *testing.T) {
	var locks Locker
	release := locks.Lock("sandbox")
	entered := make(chan struct{})
	done := make(chan struct{})

	goroutine.Go(context.Background(), func(context.Context) {
		defer close(done)
		unlock := locks.Lock("sandbox")
		defer unlock()
		close(entered)
	})

	select {
	case <-entered:
		t.Fatal("same key entered while first lock was held")
	case <-time.After(20 * time.Millisecond):
	}
	release()

	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("same key did not enter after release")
	}
	<-done
}

func TestLockerAllowsDifferentKeys(t *testing.T) {
	var locks Locker
	release := locks.Lock("a")
	defer release()

	done := make(chan struct{})
	goroutine.Go(context.Background(), func(context.Context) {
		defer close(done)
		unlock := locks.Lock("b")
		defer unlock()
	})

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("different key was blocked")
	}
}

func TestLockerBoundsConcurrentSameKey(t *testing.T) {
	var locks Locker
	var active int32
	var maxActive int32
	p := pool.New().WithMaxGoroutines(20)

	for i := 0; i < 20; i++ {
		p.Go(func() {
			unlock := locks.Lock("sandbox")
			defer unlock()
			current := atomic.AddInt32(&active, 1)
			if current > atomic.LoadInt32(&maxActive) {
				atomic.StoreInt32(&maxActive, current)
			}
			time.Sleep(time.Millisecond)
			atomic.AddInt32(&active, -1)
		})
	}
	p.Wait()

	if maxActive != 1 {
		t.Fatalf("max active = %d, want 1", maxActive)
	}
}
