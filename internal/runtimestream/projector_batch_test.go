package runtimestream

import (
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestProjectorMessageBatchFlushesAtLimitOrMaxWait(t *testing.T) {
	now := time.Date(2026, 6, 22, 15, 0, 0, 0, time.UTC)
	batch := newProjectorMessageBatch(3, time.Second)

	if !batch.empty() {
		t.Fatal("new batch should be empty")
	}
	if got := batch.readBlock(now, 5*time.Second); got != 5*time.Second {
		t.Fatalf("empty batch read block = %v, want idle block", got)
	}

	batch.add([]redis.XMessage{{ID: "1-0"}}, now)
	if batch.empty() {
		t.Fatal("batch should not be empty after add")
	}
	if batch.ready(now.Add(999 * time.Millisecond)) {
		t.Fatal("partial batch should not be ready before max wait")
	}
	if got := batch.remaining(); got != 2 {
		t.Fatalf("remaining = %d, want 2", got)
	}
	if got := batch.readBlock(now.Add(250*time.Millisecond), 5*time.Second); got != 750*time.Millisecond {
		t.Fatalf("partial batch read block = %v, want 750ms", got)
	}
	if !batch.ready(now.Add(time.Second)) {
		t.Fatal("partial batch should be ready at max wait")
	}

	batch.reset()
	batch.add([]redis.XMessage{{ID: "2-0"}, {ID: "3-0"}, {ID: "4-0"}}, now)
	if !batch.ready(now) {
		t.Fatal("full batch should be ready immediately")
	}

	batch.reset()
	if !batch.empty() || !batch.startedAt.IsZero() {
		t.Fatalf("reset batch = %+v, want empty with zero start", batch)
	}
}
