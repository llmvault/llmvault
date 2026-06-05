package gateway

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestServiceReceiveWebhookUsesMainEmployeeRuntimeSandbox(t *testing.T) {
	db := connectGatewayTestDB(t)
	route := seedGatewayRoute(t, db)

	employeeImage := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.5"
	oldEmployeeImage := "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.1.4"
	specialistImage := "ghcr.io/usehivy/hivy-sandboxes-runtime-specialist:v3.1.5"

	var employeeSandbox model.Sandbox
	if err := db.Where("org_id = ? AND employee_id = ?", route.OrgID, route.EmployeeID).First(&employeeSandbox).Error; err != nil {
		t.Fatalf("load seeded sandbox: %v", err)
	}
	if err := db.Model(&employeeSandbox).Update("snapshot_id", oldEmployeeImage).Error; err != nil {
		t.Fatalf("set employee sandbox image: %v", err)
	}
	specialistSandbox := model.Sandbox{
		OrgID:                  &route.OrgID,
		EmployeeID:             &route.EmployeeID,
		SnapshotID:             &specialistImage,
		ExternalID:             "gateway-specialist-" + uuid.NewString(),
		RuntimeURL:             "http://localhost:2",
		EncryptedRuntimeSecret: []byte("specialist-key"),
		Status:                 "running",
	}
	if err := db.Create(&specialistSandbox).Error; err != nil {
		t.Fatalf("create specialist sandbox: %v", err)
	}
	existingBadSession := model.EmployeeSession{
		OrgID:                 route.OrgID,
		EmployeeID:            route.EmployeeID,
		SandboxID:             specialistSandbox.ID,
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
	service.SetRuntimeImages(employeeImage, specialistImage)

	result, err := service.ReceiveWebhook(t.Context(), WebhookEnvelope{
		RouteID: route.ID,
		Body:    fakeSlackBody("200.000", "", ""),
	})
	if err != nil {
		t.Fatalf("receive webhook: %v", err)
	}
	if result.Session.SandboxID != employeeSandbox.ID {
		t.Fatalf("session sandbox = %s, want employee runtime sandbox %s", result.Session.SandboxID, employeeSandbox.ID)
	}
	var stored model.EmployeeSession
	if err := db.First(&stored, "id = ?", existingBadSession.ID).Error; err != nil {
		t.Fatalf("load retargeted session: %v", err)
	}
	if stored.SandboxID != employeeSandbox.ID {
		t.Fatalf("stored session sandbox = %s, want retargeted employee runtime sandbox %s", stored.SandboxID, employeeSandbox.ID)
	}
}
