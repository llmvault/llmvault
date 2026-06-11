package middleware

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hibiken/asynq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

// fakeEnqueuer records the tasks it is asked to enqueue. It satisfies
// enqueue.TaskEnqueuer for the durable-spill fallback path.
type fakeEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
	err   error
}

func (f *fakeEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	return f.EnqueueContext(context.Background(), task)
}

func (f *fakeEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.err != nil {
		return nil, f.err
	}
	f.tasks = append(f.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func (f *fakeEnqueuer) Close() error { return nil }

func (f *fakeEnqueuer) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.tasks)
}

func connectGenTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres (run `make test-setup`): %v", err)
	}
	testdb.ApplyMigrations(t, db)
	t.Cleanup(func() {
		if sqlDB, err := db.DB(); err == nil {
			_ = sqlDB.Close()
		}
	})
	return db
}

// brokenDB returns a gorm handle whose underlying connection is closed, so every
// write fails — used to exercise the flush-failure fallback deterministically.
func brokenDB(t *testing.T) *gorm.DB {
	t.Helper()
	db := connectGenTestDB(t)
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get sql db: %v", err)
	}
	if err := sqlDB.Close(); err != nil {
		t.Fatalf("close sql db: %v", err)
	}
	return db
}

// TestGenerationWriter_FlushFailureSpillsToAsynq exercises P0-9: when the DB
// insert keeps failing, the batch is not silently dropped — each row is spilled
// to the durable asynq generation:write task.
func TestGenerationWriter_FlushFailureSpillsToAsynq(t *testing.T) {
	enq := &fakeEnqueuer{}

	gw := &GenerationWriter{
		db:            brokenDB(t),
		entries:       make(chan model.Generation, 16),
		flushInterval: 10 * time.Millisecond,
	}
	gw.SetEnqueuer(enq)
	gw.wg.Add(1)
	go gw.drain(context.Background())

	const n = 3
	for i := 0; i < n; i++ {
		gw.Write(context.Background(), model.Generation{ID: "gen_flush_" + string(rune('a'+i))})
	}

	gw.Shutdown(context.Background())

	if got := enq.count(); got != n {
		t.Fatalf("spilled tasks = %d, want %d (rows must not be dropped on flush failure)", got, n)
	}
}

// TestGenerationWriter_BufferFullSpillsToAsynq exercises the backpressure path:
// when the in-memory channel is full, Write spills the billing-bearing entry to
// asynq rather than dropping it.
func TestGenerationWriter_BufferFullSpillsToAsynq(t *testing.T) {
	enq := &fakeEnqueuer{}

	// No drain goroutine, buffer size 1: the first Write fills the channel, the
	// second has nowhere to go and must spill.
	gw := &GenerationWriter{
		entries:       make(chan model.Generation, 1),
		flushInterval: time.Second,
	}
	gw.SetEnqueuer(enq)

	gw.Write(context.Background(), model.Generation{ID: "gen_buf_1"}) // buffered
	gw.Write(context.Background(), model.Generation{ID: "gen_buf_2"}) // buffer full -> spill

	if got := enq.count(); got != 1 {
		t.Fatalf("spilled tasks = %d, want 1 (buffer-full entry must spill, not drop)", got)
	}
}

// TestGenerationWriter_ShutdownTimeoutSpillsQueue exercises the shutdown path:
// if the drain cannot finish before the deadline, queued entries are spilled to
// asynq instead of being abandoned.
func TestGenerationWriter_ShutdownTimeoutSpillsQueue(t *testing.T) {
	enq := &fakeEnqueuer{}

	gw := &GenerationWriter{
		entries:       make(chan model.Generation, 8),
		flushInterval: time.Second,
	}
	gw.SetEnqueuer(enq)

	// Hold the WaitGroup with a goroutine that stays blocked until we release it.
	// This guarantees Shutdown's wg.Wait never completes before the (already
	// expired) deadline, so the ctx.Done branch — and the spill loop — runs
	// deterministically. This models a drain goroutine that raced the deadline.
	release := make(chan struct{})
	gw.wg.Add(1)
	go func() {
		<-release
		gw.wg.Done()
	}()

	const n = 4
	for i := 0; i < n; i++ {
		gw.entries <- model.Generation{ID: "gen_sd_" + string(rune('a'+i))}
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already expired -> Shutdown takes the timeout branch

	gw.Shutdown(ctx)
	close(release)

	if got := enq.count(); got != n {
		t.Fatalf("spilled tasks = %d, want %d (shutdown must spill remaining queue)", got, n)
	}
}
