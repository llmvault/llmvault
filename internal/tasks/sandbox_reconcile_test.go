package tasks

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func seedReconcileSandbox(t *testing.T, db *gorm.DB, ext, status string) {
	t.Helper()
	sb := model.Sandbox{
		ProviderID:             sandbox.ProviderMicrosandbox,
		ExternalID:             ext,
		RuntimeURL:             "http://runtime.test",
		EncryptedRuntimeSecret: []byte("x"),
		Status:                 status,
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("seed sandbox %s: %v", ext, err)
	}
	t.Cleanup(func() { db.Where("external_id = ?", ext).Delete(&model.Sandbox{}) })
}

func statusOf(t *testing.T, db *gorm.DB, ext string) string {
	t.Helper()
	var sb model.Sandbox
	if err := db.First(&sb, "external_id = ?", ext).Error; err != nil {
		t.Fatalf("load sandbox %s: %v", ext, err)
	}
	return sb.Status
}

func TestReconcileSandboxStates(t *testing.T) {
	db := connectTestDB(t)
	ctx := context.Background()
	p := "reconcile-" + uuid.NewString()[:8] + "-"

	// desync both directions, an in-flight row, and a wrong-provider row.
	seedReconcileSandbox(t, db, p+"stranded", "stopped")  // CP says running -> fix
	seedReconcileSandbox(t, db, p+"gone", "running")      // CP says stopped -> fix
	seedReconcileSandbox(t, db, p+"same", "running")      // CP says running -> no-op
	seedReconcileSandbox(t, db, p+"creating", "creating") // in-flight -> never touched
	seedReconcileSandbox(t, db, p+"error", "error")       // terminal -> never touched

	// A docker sandbox must be ignored by the microsandbox reconcile pass.
	docker := model.Sandbox{
		ProviderID: sandbox.ProviderDocker, ExternalID: p + "docker",
		RuntimeURL: "http://x", EncryptedRuntimeSecret: []byte("x"), Status: "stopped",
	}
	if err := db.Create(&docker).Error; err != nil {
		t.Fatalf("seed docker: %v", err)
	}
	t.Cleanup(func() { db.Where("external_id = ?", p+"docker").Delete(&model.Sandbox{}) })

	states := []sandbox.SandboxState{
		{ExternalID: p + "stranded", Status: sandbox.StatusRunning},
		{ExternalID: p + "gone", Status: sandbox.StatusStopped},
		{ExternalID: p + "same", Status: sandbox.StatusRunning},
		{ExternalID: p + "creating", Status: sandbox.StatusRunning},   // must not override in-flight
		{ExternalID: p + "error", Status: sandbox.StatusStopped},      // must not override terminal
		{ExternalID: p + "docker", Status: sandbox.StatusRunning},     // wrong provider, must skip
		{ExternalID: p + "ephemeral", Status: sandbox.StatusCreating}, // non-liveness target, skipped
	}

	n, err := reconcileSandboxStates(ctx, db, sandbox.ProviderMicrosandbox, states)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	// Only the two genuine desyncs change.
	if n != 2 {
		t.Fatalf("reconciled = %d, want 2", n)
	}
	if got := statusOf(t, db, p+"stranded"); got != "running" {
		t.Fatalf("stranded status = %q, want running", got)
	}
	if got := statusOf(t, db, p+"gone"); got != "stopped" {
		t.Fatalf("gone status = %q, want stopped", got)
	}
	if got := statusOf(t, db, p+"creating"); got != "creating" {
		t.Fatalf("creating was clobbered: %q", got)
	}
	if got := statusOf(t, db, p+"error"); got != "error" {
		t.Fatalf("error was clobbered: %q", got)
	}
	if got := statusOf(t, db, p+"docker"); got != "stopped" {
		t.Fatalf("docker sandbox was touched by microsandbox pass: %q", got)
	}
}
