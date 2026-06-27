package tasks

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type stubOrgHivyAgentEnsurer struct {
	calls int
	orgID uuid.UUID
	err   error
}

func (s *stubOrgHivyAgentEnsurer) EnsureOrgHivyAgent(_ context.Context, orgID uuid.UUID) error {
	s.calls++
	s.orgID = orgID
	return s.err
}

func TestOrgHivyAgentProvisionHandlerEnsuresOrgAgent(t *testing.T) {
	orgID := uuid.New()
	task, _, err := NewOrgHivyAgentProvisionTask(OrgHivyAgentProvisionPayload{OrgID: orgID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	ensurer := &stubOrgHivyAgentEnsurer{}

	err = NewOrgHivyAgentProvisionHandler(ensurer).Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if ensurer.calls != 1 {
		t.Fatalf("ensure calls = %d, want 1", ensurer.calls)
	}
	if ensurer.orgID != orgID {
		t.Fatalf("org id = %s, want %s", ensurer.orgID, orgID)
	}
}

func TestOrgHivyAgentProvisionHandlerReturnsEnsureError(t *testing.T) {
	orgID := uuid.New()
	task, _, err := NewOrgHivyAgentProvisionTask(OrgHivyAgentProvisionPayload{OrgID: orgID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	want := errors.New("runner unavailable")

	err = NewOrgHivyAgentProvisionHandler(&stubOrgHivyAgentEnsurer{err: want}).Handle(context.Background(), task)
	if !errors.Is(err, want) {
		t.Fatalf("handle error = %v, want %v", err, want)
	}
}

func TestNewOrgHivyAgentProvisionTaskRejectsEmptyOrgID(t *testing.T) {
	_, _, err := NewOrgHivyAgentProvisionTask(OrgHivyAgentProvisionPayload{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestNewOrgHivyAgentProvisionTaskUsesDefaultQueue(t *testing.T) {
	task, opts, err := NewOrgHivyAgentProvisionTask(OrgHivyAgentProvisionPayload{OrgID: uuid.New()})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	if task.Type() != TypeOrgHivyAgentProvision {
		t.Fatalf("task type = %q, want %q", task.Type(), TypeOrgHivyAgentProvision)
	}
	for _, opt := range opts {
		if opt.Type() == asynq.QueueOpt && opt.Value() == QueueDefault {
			return
		}
	}
	t.Fatalf("expected queue option %q", QueueDefault)
}
