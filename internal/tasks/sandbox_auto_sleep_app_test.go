package tasks

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// seedAppSandbox creates a running microsandbox app sandbox with the given
// activity timestamps and an apps row pointing at it. Returns the sandbox ID.
func seedAppSandbox(t *testing.T, db *gorm.DB, orgID, channelID, sheetID uuid.UUID, appActivity, gwActivity *time.Time) uuid.UUID {
	t.Helper()
	sb := model.Sandbox{
		OrgID: &orgID, ProviderID: sandbox.ProviderMicrosandbox,
		ExternalID: "app-sb-" + uuid.NewString()[:8], RuntimeURL: "http://x",
		EncryptedRuntimeSecret: []byte("x"), Status: "running",
		LastAppActivityAt: appActivity, LastGatewayActivityAt: gwActivity,
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("seed app sandbox: %v", err)
	}
	app := model.App{
		OrgID: orgID, ChannelID: channelID, SheetID: sheetID,
		Slug: "a" + uuid.NewString()[:8], Name: "app", EncryptedAppSecret: []byte("x"),
		SandboxID: &sb.ID, Status: model.AppStatusRunning,
	}
	if err := db.Create(&app).Error; err != nil {
		t.Fatalf("seed app: %v", err)
	}
	t.Cleanup(func() {
		db.Where("id = ?", app.ID).Delete(&model.App{})
		db.Where("id = ?", sb.ID).Delete(&model.Sandbox{})
	})
	return sb.ID
}

func TestIdleAppSandboxes(t *testing.T) {
	db := connectTestDB(t)
	ctx := context.Background()
	orgID, agentID, channelID := watchdogSeedOrg(t, db)
	_ = agentID
	sheet := model.Sheet{OrgID: orgID, ChannelID: channelID, Slug: "s" + uuid.NewString()[:8], Name: "S"}
	if err := db.Create(&sheet).Error; err != nil {
		t.Fatalf("seed sheet: %v", err)
	}

	old := time.Now().Add(-2 * time.Hour)
	recent := time.Now().Add(-1 * time.Minute)
	cutoff := time.Now().Add(-autoSleepIdleThreshold)

	idle := seedAppSandbox(t, db, orgID, channelID, sheet.ID, &old, nil)          // stale app ping -> idle
	recentApp := seedAppSandbox(t, db, orgID, channelID, sheet.ID, &recent, nil)  // recent ping -> awake
	gwFallback := seedAppSandbox(t, db, orgID, channelID, sheet.ID, nil, &recent) // no ping but recent gateway -> awake

	got, err := NewSandboxAutoSleepHandler(db, nil).idleAppSandboxes(ctx, cutoff)
	if err != nil {
		t.Fatalf("idleAppSandboxes: %v", err)
	}
	ids := map[uuid.UUID]bool{}
	for _, s := range got {
		ids[s.ID] = true
	}
	if !ids[idle] {
		t.Fatal("stale app sandbox was not selected for sleep")
	}
	if ids[recentApp] {
		t.Fatal("recently-active app was selected for sleep")
	}
	if ids[gwFallback] {
		t.Fatal("app with recent gateway activity (fallback) was selected for sleep")
	}
}

func TestMirrorGatewayActivity(t *testing.T) {
	db := connectTestDB(t)
	ctx := context.Background()
	ext := "gw-mirror-" + uuid.NewString()[:8]
	seedReconcileSandbox(t, db, ext, "running")

	ts := time.Now().Add(-3 * time.Minute).UTC().Truncate(time.Second)
	states := []sandbox.SandboxState{
		{ExternalID: ext, Status: sandbox.StatusRunning, LastGatewayActivityAt: &ts},
		{ExternalID: "", Status: sandbox.StatusRunning},     // skipped
		{ExternalID: "nope", Status: sandbox.StatusRunning}, // no gw ts -> skipped
	}
	if err := mirrorGatewayActivity(ctx, db, sandbox.ProviderMicrosandbox, states); err != nil {
		t.Fatalf("mirror: %v", err)
	}
	var sb model.Sandbox
	if err := db.First(&sb, "external_id = ?", ext).Error; err != nil {
		t.Fatalf("load: %v", err)
	}
	if sb.LastGatewayActivityAt == nil || !sb.LastGatewayActivityAt.UTC().Truncate(time.Second).Equal(ts) {
		t.Fatalf("last_gateway_activity_at = %v, want %v", sb.LastGatewayActivityAt, ts)
	}
}
