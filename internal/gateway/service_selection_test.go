package gateway

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestServiceReceiveWebhookUsesLatestAgentRuntimeSandbox(t *testing.T) {
	db := connectGatewayTestDB(t)
	route := seedGatewayRoute(t, db)

	employeeImage := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.5"
	oldEmployeeImage := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.4"

	var employeeSandbox model.Sandbox
	if err := db.Where("org_id = ? AND employee_id = ?", route.OrgID, route.EmployeeID).First(&employeeSandbox).Error; err != nil {
		t.Fatalf("load seeded sandbox: %v", err)
	}
	if err := db.Model(&employeeSandbox).Update("snapshot_id", oldEmployeeImage).Error; err != nil {
		t.Fatalf("set employee sandbox image: %v", err)
	}
	replacementSandbox := model.Sandbox{
		OrgID:                  &route.OrgID,
		EmployeeID:             &route.EmployeeID,
		SnapshotID:             &employeeImage,
		ExternalID:             "gateway-agent-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:2",
		EncryptedRuntimeSecret: []byte("agent-key"),
		Status:                 "running",
	}
	if err := db.Create(&replacementSandbox).Error; err != nil {
		t.Fatalf("create replacement sandbox: %v", err)
	}
	existingBadSession := model.EmployeeSession{
		OrgID:                 route.OrgID,
		EmployeeID:            route.EmployeeID,
		SandboxID:             employeeSandbox.ID,
		RuntimeConversationID: "runtime-bad-existing",
		Source:                Source,
		SourceID:              &route.ID,
		SourceResourceKey:     "fake-slack:T123:C123:200.000",
		Status:                "active",
		Name:                  "Gateway: fake-slack:T123:C123:200.000",
		IntegrationScopes:     model.JSON{},
	}
	if err := db.Create(&existingBadSession).Error; err != nil {
		t.Fatalf("create existing bad session: %v", err)
	}

	runtime := &recordingRuntime{}
	service := NewService(db, runtime, nil, NewFakeSlackAdapter())
	service.SetRuntimeImages(employeeImage)

	result, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("200.000", "", ""),
	})
	if err != nil {
		t.Fatalf("receive webhook: %v", err)
	}
	if result.Session.SandboxID != replacementSandbox.ID {
		t.Fatalf("session sandbox = %s, want agent runtime sandbox %s", result.Session.SandboxID, replacementSandbox.ID)
	}
	var stored model.EmployeeSession
	if err := db.First(&stored, "id = ?", existingBadSession.ID).Error; err != nil {
		t.Fatalf("load retargeted session: %v", err)
	}
	if stored.SandboxID != replacementSandbox.ID {
		t.Fatalf("stored session sandbox = %s, want retargeted agent runtime sandbox %s", stored.SandboxID, replacementSandbox.ID)
	}
}
