package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

func TestWarmPoolReconcileCreatesWarmSlotAndClaimMarksClaiming(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{})
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(health.Close)
	provider.warmEndpoint = health.URL

	pool := NewWarmPool(db, provider, testEncKey(t), &config.Config{
		SandboxWarmPoolEmployeeSize: 1,
		RailwayRuntimePort:          7080,
		SandboxesRuntimeBaseImage:   "runtime:test",
		Environment:                 "production",
		SentryTracesSampleRate:      0.25,
		AgentSandboxSentryDSN:       "https://agent@example.com/2",
	})
	if pool == nil {
		t.Fatal("warm pool is nil")
	}

	created, err := pool.Reconcile(context.Background(), model.SandboxWarmSlotModeEmployee, nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created slots = %d, want 1", len(created))
	}
	if len(provider.warmCreateCalls) != 1 {
		t.Fatalf("warm create calls = %d, want 1", len(provider.warmCreateCalls))
	}
	warmEnv := provider.warmCreateCalls[0].EnvVars
	if got := warmEnv["SENTRY_DSN"]; got != "https://agent@example.com/2" {
		t.Fatalf("warm slot sentry dsn = %q, want agent sandbox dsn", got)
	}
	if got := warmEnv["SENTRY_ENVIRONMENT"]; got != "production" {
		t.Fatalf("warm slot sentry environment = %q", got)
	}
	if got := warmEnv["SENTRY_ENABLE_LOGS"]; got != "true" {
		t.Fatalf("warm slot sentry enable logs = %q", got)
	}
	result, err := pool.CheckWarmSlot(context.Background(), created[0])
	if err != nil {
		t.Fatalf("check warm slot: %v", err)
	}
	if result == nil || !result.Ready {
		t.Fatalf("check result = %#v, want ready", result)
	}

	org := createTestOrg(t, db)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "pending",
		RuntimeURL:             "pending",
		EncryptedRuntimeSecret: []byte("encrypted"),
		Status:                 "creating",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	claimed, err := pool.Claim(context.Background(), model.SandboxWarmSlotModeEmployee, sb.ID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.EndpointURL != health.URL {
		t.Fatalf("endpoint = %q, want %q", claimed.EndpointURL, health.URL)
	}

	var slot model.SandboxWarmSlot
	if err := db.First(&slot, "id = ?", claimed.ID).Error; err != nil {
		t.Fatalf("load slot: %v", err)
	}
	if slot.Status != model.SandboxWarmSlotStatusClaiming {
		t.Fatalf("slot status = %q, want claiming", slot.Status)
	}
	if slot.ClaimedSandboxID == nil || *slot.ClaimedSandboxID != sb.ID {
		t.Fatalf("claimed sandbox id = %v, want %s", slot.ClaimedSandboxID, sb.ID)
	}
}

func TestWarmPoolDeletesStaleRuntimeImageSlotsAndReconciles(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()
	if err := db.Where("provider_id = ?", provider.ID()).Delete(&model.SandboxWarmSlot{}).Error; err != nil {
		t.Fatalf("clean warm slots: %v", err)
	}
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/healthz" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(health.Close)
	provider.warmEndpoint = health.URL
	encKey := testEncKey(t)
	secret, err := encKey.EncryptString("warm-secret")
	if err != nil {
		t.Fatalf("encrypt warm secret: %v", err)
	}
	stale := model.SandboxWarmSlot{
		ProviderID:             provider.ID(),
		Mode:                   model.SandboxWarmSlotModeEmployee,
		Status:                 model.SandboxWarmSlotStatusWarm,
		ExternalID:             "stale-warm-slot",
		EndpointURL:            "https://stale.example",
		RuntimeImage:           "runtime:test-v1",
		RuntimePort:            7080,
		EncryptedRuntimeSecret: secret,
	}
	if err := db.Create(&stale).Error; err != nil {
		t.Fatalf("create stale slot: %v", err)
	}

	pool := NewWarmPool(db, provider, encKey, &config.Config{
		SandboxWarmPoolEmployeeSize: 1,
		RailwayRuntimePort:          7080,
		SandboxesRuntimeBaseImage:   "runtime:test-v2",
	})
	created, err := pool.Reconcile(context.Background(), model.SandboxWarmSlotModeEmployee, nil)
	if err != nil {
		t.Fatalf("reconcile: %v", err)
	}
	if len(created) != 1 {
		t.Fatalf("created slots = %d, want 1 current-image slot", len(created))
	}
	var staleSlot model.SandboxWarmSlot
	if err := db.First(&staleSlot, "id = ?", stale.ID).Error; err != nil {
		t.Fatalf("load stale slot: %v", err)
	}
	if staleSlot.Status != model.SandboxWarmSlotStatusError {
		t.Fatalf("stale slot status = %q, want error", staleSlot.Status)
	}
	if staleSlot.ErrorMessage == nil || !strings.Contains(*staleSlot.ErrorMessage, "stale runtime image") {
		t.Fatalf("stale slot error message = %v, want stale runtime image", staleSlot.ErrorMessage)
	}
	if len(provider.deletedIDs) != 1 || provider.deletedIDs[0] != stale.ExternalID {
		t.Fatalf("deleted IDs = %#v, want stale external ID", provider.deletedIDs)
	}
	if got := provider.warmCreateCalls[0].RuntimeImage; got != "runtime:test-v2" {
		t.Fatalf("created runtime image = %q, want runtime:test-v2", got)
	}
	if _, err := pool.CheckWarmSlot(context.Background(), created[0]); err != nil {
		t.Fatalf("check new warm slot: %v", err)
	}

	org := createTestOrg(t, db)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "pending",
		RuntimeURL:             "pending",
		EncryptedRuntimeSecret: []byte("encrypted"),
		Status:                 "creating",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	claimed, err := pool.Claim(context.Background(), model.SandboxWarmSlotModeEmployee, sb.ID)
	if err != nil {
		t.Fatalf("claim: %v", err)
	}
	if claimed.ID == stale.ID {
		t.Fatalf("claimed stale slot %s", stale.ID)
	}
	var claimedSlot model.SandboxWarmSlot
	if err := db.First(&claimedSlot, "id = ?", claimed.ID).Error; err != nil {
		t.Fatalf("load claimed slot: %v", err)
	}
	if claimedSlot.RuntimeImage != "runtime:test-v2" {
		t.Fatalf("claimed runtime image = %q, want runtime:test-v2", claimedSlot.RuntimeImage)
	}
}
