package tasks

import (
	"context"
	"encoding/json"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

type sandboxSleepRecordingEnqueuer struct {
	mu    sync.Mutex
	tasks []*asynq.Task
}

func TestSandboxAutoSleepQueriesExcludeDesktopRuntimes(t *testing.T) {
	db, err := gorm.Open(postgres.New(postgres.Config{
		DSN: "host=localhost user=hivy dbname=hivy sslmode=disable",
	}), &gorm.Config{DryRun: true, DisableAutomaticPing: true})
	if err != nil {
		t.Fatalf("open dry-run database: %v", err)
	}
	handler := NewSandboxAutoSleepHandler(db, nil, time.Minute)
	queries := []*gorm.DB{
		handler.idleAgentSandboxesQuery(t.Context(), time.Now()),
		handler.idleAppSandboxesQuery(t.Context(), time.Now()),
	}
	for _, query := range queries {
		statement := query.Find(&[]model.Sandbox{}).Statement
		if !strings.Contains(statement.SQL.String(), "provider_id") {
			t.Fatalf("desktop provider filter missing from query: %s", statement.SQL.String())
		}
		found := false
		for _, value := range statement.Vars {
			if value == sandbox.ProviderDesktop {
				found = true
				break
			}
		}
		if !found {
			t.Fatalf("desktop provider value missing from query vars: %#v", statement.Vars)
		}
	}
}

func (e *sandboxSleepRecordingEnqueuer) Enqueue(task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	return e.EnqueueContext(context.Background(), task)
}

func (e *sandboxSleepRecordingEnqueuer) EnqueueContext(_ context.Context, task *asynq.Task, _ ...asynq.Option) (*asynq.TaskInfo, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.tasks = append(e.tasks, task)
	return &asynq.TaskInfo{}, nil
}

func (*sandboxSleepRecordingEnqueuer) Close() error { return nil }

func TestSandboxAutoSleepDispatchesOneTaskPerCandidate(t *testing.T) {
	enqueuer := &sandboxSleepRecordingEnqueuer{}
	handler := NewSandboxAutoSleepHandler(nil, enqueuer, 0)
	candidates := make([]sandboxSleepCandidate, 0, 100)
	want := make(map[uuid.UUID]struct{}, 100)
	for i := 0; i < 100; i++ {
		id := uuid.New()
		want[id] = struct{}{}
		candidates = append(candidates, sandboxSleepCandidate{
			Sandbox: model.Sandbox{ID: id},
			Kind:    sandboxSleepKindAgent,
		})
	}

	enqueued, err := handler.enqueueCandidates(t.Context(), candidates)
	if err != nil {
		t.Fatalf("enqueue candidates: %v", err)
	}
	if enqueued != 100 || len(enqueuer.tasks) != 100 {
		t.Fatalf("enqueued=%d recorded=%d want 100", enqueued, len(enqueuer.tasks))
	}
	for _, task := range enqueuer.tasks {
		if task.Type() != TypeSandboxSleep {
			t.Fatalf("task type=%q want %q", task.Type(), TypeSandboxSleep)
		}
		var payload SandboxSleepPayload
		if err := json.Unmarshal(task.Payload(), &payload); err != nil {
			t.Fatalf("decode payload: %v", err)
		}
		if _, ok := want[payload.SandboxID]; !ok {
			t.Fatalf("unexpected sandbox id %s", payload.SandboxID)
		}
		delete(want, payload.SandboxID)
	}
	if len(want) != 0 {
		t.Fatalf("missing %d sandbox tasks", len(want))
	}
}

func TestNewSandboxSleepTaskUsesDedicatedQueue(t *testing.T) {
	task, opts, err := NewSandboxSleepTask(SandboxSleepPayload{SandboxID: uuid.New(), Kind: sandboxSleepKindApp})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if task.Type() != TypeSandboxSleep {
		t.Fatalf("task type=%q", task.Type())
	}
	var queue string
	for _, opt := range opts {
		if opt.Type() == asynq.QueueOpt {
			queue, _ = opt.Value().(string)
		}
	}
	if queue != QueueSandboxLifecycle {
		t.Fatalf("queue=%q want %q", queue, QueueSandboxLifecycle)
	}
}
