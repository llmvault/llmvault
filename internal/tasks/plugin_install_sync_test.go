package tasks

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

func TestNewPluginInstallSyncTaskUsesDefaultQueue(t *testing.T) {
	payload := PluginInstallSyncPayload{
		OrgID:     uuid.New(),
		PluginID:  uuid.New(),
		InstallID: uuid.New(),
	}
	task, opts, err := NewPluginInstallSyncTask(payload)
	if err != nil {
		t.Fatalf("NewPluginInstallSyncTask: %v", err)
	}
	if task.Type() != TypePluginInstallSync {
		t.Fatalf("task type = %q, want %q", task.Type(), TypePluginInstallSync)
	}
	var decoded PluginInstallSyncPayload
	if err := json.Unmarshal(task.Payload(), &decoded); err != nil {
		t.Fatalf("decode payload: %v", err)
	}
	if decoded != payload {
		t.Fatalf("payload = %+v, want %+v", decoded, payload)
	}
	for _, opt := range opts {
		if opt.Type() == asynq.QueueOpt && opt.Value() == QueueDefault {
			return
		}
	}
	t.Fatalf("expected queue option %q", QueueDefault)
}

func TestNewPluginInstallSyncTaskRejectsMissingIDs(t *testing.T) {
	valid := PluginInstallSyncPayload{OrgID: uuid.New(), PluginID: uuid.New(), InstallID: uuid.New()}
	cases := []PluginInstallSyncPayload{
		{PluginID: valid.PluginID, InstallID: valid.InstallID},
		{OrgID: valid.OrgID, InstallID: valid.InstallID},
		{OrgID: valid.OrgID, PluginID: valid.PluginID},
	}
	for _, tc := range cases {
		if _, _, err := NewPluginInstallSyncTask(tc); err == nil {
			t.Fatalf("expected error for payload %+v", tc)
		}
	}
}
