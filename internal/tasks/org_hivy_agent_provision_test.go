package tasks

import (
	"context"
	"errors"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type stubOrgHivyAgentSyncer struct {
	calls int
	orgID uuid.UUID
	err   error
}

func (s *stubOrgHivyAgentSyncer) SyncOrgHivyAgent(_ context.Context, orgID uuid.UUID) error {
	s.calls++
	s.orgID = orgID
	return s.err
}

func TestOrgHivyAgentProvisionHandlerSyncsOrg(t *testing.T) {
	orgID := uuid.New()
	task, _, err := NewOrgHivyAgentProvisionTask(OrgHivyAgentProvisionPayload{OrgID: orgID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	syncer := &stubOrgHivyAgentSyncer{}

	err = NewOrgHivyAgentProvisionHandler(syncer).Handle(context.Background(), task)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if syncer.calls != 1 {
		t.Fatalf("sync calls = %d, want 1", syncer.calls)
	}
	if syncer.orgID != orgID {
		t.Fatalf("org id = %s, want %s", syncer.orgID, orgID)
	}
}

func TestOrgHivyAgentProvisionHandlerReturnsSyncError(t *testing.T) {
	orgID := uuid.New()
	task, _, err := NewOrgHivyAgentProvisionTask(OrgHivyAgentProvisionPayload{OrgID: orgID})
	if err != nil {
		t.Fatalf("new task: %v", err)
	}
	want := errors.New("runner unavailable")

	err = NewOrgHivyAgentProvisionHandler(&stubOrgHivyAgentSyncer{err: want}).Handle(context.Background(), task)
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
