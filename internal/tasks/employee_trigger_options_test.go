package tasks

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func queueFromOptions(opts []asynq.Option) (string, bool) {
	for _, opt := range opts {
		if opt.Type() == asynq.QueueOpt {
			if name, ok := opt.Value().(string); ok {
				return name, true
			}
		}
	}
	return "", false
}

// TestTriggerDispatchTaskOptionsAreReturnedSeparately guards P0-11: the
// critical-queue routing/retry/timeout must be returned as options (not baked
// into the task), so they are passed at enqueue time and survive the enqueue
// client's Sentry trace-payload rewrite. A task carrying baked options would be
// silently demoted to the default queue when the payload is rewritten.
func TestTriggerDispatchTaskOptionsAreReturnedSeparately(t *testing.T) {
	task, opts, err := NewEmployeeTriggerDispatchTask(EmployeeTriggerDispatchPayload{
		OrgID:      uuid.New(),
		DeliveryID: "delivery-1",
	})
	if err != nil {
		t.Fatalf("build task: %v", err)
	}
	if task.Type() != TypeEmployeeTriggerDispatch {
		t.Fatalf("task type = %q", task.Type())
	}
	queue, ok := queueFromOptions(opts)
	if !ok || queue != QueueCritical {
		t.Fatalf("expected critical queue option, got %q (present=%v)", queue, ok)
	}
}

// TestTriggerDispatchTaskBuilderReturnsOptions guards the failed-event retry
// path: the registered TaskBuilder must surface the options so RetryFailedEvent
// re-applies the critical queue/retry/timeout instead of asynq defaults (P0-11).
func TestTriggerDispatchTaskBuilderReturnsOptions(t *testing.T) {
	builder, ok := lookupTaskBuilder(TypeEmployeeTriggerDispatch)
	if !ok {
		t.Fatalf("no task builder registered for %s", TypeEmployeeTriggerDispatch)
	}
	payload, err := json.Marshal(EmployeeTriggerDispatchPayload{OrgID: uuid.New(), DeliveryID: "d2"})
	if err != nil {
		t.Fatalf("marshal payload: %v", err)
	}
	_, opts, err := builder(payload)
	if err != nil {
		t.Fatalf("builder: %v", err)
	}
	queue, ok := queueFromOptions(opts)
	if !ok || queue != QueueCritical {
		t.Fatalf("builder options missing critical queue, got %q (present=%v)", queue, ok)
	}
}
