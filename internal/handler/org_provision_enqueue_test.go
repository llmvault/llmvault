package handler_test

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/tasks"
)

func assertOrgProvisionTaskEnqueued(t *testing.T, enq *enqueue.MockClient) uuid.UUID {
	t.Helper()
	for _, task := range enq.Tasks() {
		if task.TypeName != tasks.TypeOrgHivyAgentProvision {
			continue
		}
		var payload tasks.OrgHivyAgentProvisionPayload
		if err := json.Unmarshal(task.Payload, &payload); err != nil {
			t.Fatalf("decode org provision payload: %v", err)
		}
		if payload.OrgID == uuid.Nil {
			t.Fatal("org provision payload org_id is empty")
		}
		return payload.OrgID
	}
	t.Fatalf("expected %s task to be enqueued", tasks.TypeOrgHivyAgentProvision)
	return uuid.Nil
}
