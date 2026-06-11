package sandbox

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/model"
)

// TestWakeSandboxFlipsRunningOnlyAfterHealthy verifies P2-35: the row must not
// be persisted as 'running' until waitForEmployeeRuntimeLive confirms the
// runtime is healthy. Otherwise a concurrent EnsureSandboxActive would observe
// 'running' and route traffic to a sandbox that never actually came back up.
func TestWakeSandboxFlipsRunningOnlyAfterHealthy(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()

	// healthy only flips true once the test allows the probe to succeed; until
	// then /healthz returns 503 so the wake stays blocked in the health wait.
	healthy := make(chan struct{})
	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		select {
		case <-healthy:
			w.WriteHeader(http.StatusOK)
		default:
			w.WriteHeader(http.StatusServiceUnavailable)
		}
	}))
	t.Cleanup(health.Close)
	provider.endpointOverride = health.URL

	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})

	org := createTestOrg(t, db)
	provider.registerSandbox("wake-ext", StatusStopped)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "wake-ext",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 string(StatusStopped),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	type result struct {
		out *model.Sandbox
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := orch.WakeSandbox(context.Background(), &sb)
		done <- result{out, err}
	}()

	// While the runtime is still unhealthy, the persisted status must not be
	// 'running'. Give the wake a beat to run StartSandbox + RefreshURL + begin
	// the health wait.
	time.Sleep(150 * time.Millisecond)
	var midflight model.Sandbox
	if err := db.First(&midflight, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload mid-flight: %v", err)
	}
	if midflight.Status == string(StatusRunning) {
		t.Fatalf("status persisted as running before health wait completed; got %q", midflight.Status)
	}

	// Now allow the runtime to report healthy; the wake should complete and
	// flip to running.
	close(healthy)

	select {
	case res := <-done:
		if res.err != nil {
			t.Fatalf("WakeSandbox: %v", res.err)
		}
		if res.out.Status != string(StatusRunning) {
			t.Fatalf("in-memory status = %q, want running", res.out.Status)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("WakeSandbox did not complete after runtime became healthy")
	}

	var final model.Sandbox
	if err := db.First(&final, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload final: %v", err)
	}
	if final.Status != string(StatusRunning) {
		t.Fatalf("persisted status = %q, want running after healthy", final.Status)
	}
}

// TestWakeSandboxMarksErrorWhenRuntimeNeverHealthy verifies the failure side of
// P2-35: when the runtime never becomes healthy the row must end in 'error',
// never left asserting 'running'.
func TestWakeSandboxMarksErrorWhenRuntimeNeverHealthy(t *testing.T) {
	db := setupTestDB(t)
	provider := newMockProvider()

	health := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(health.Close)
	provider.endpointOverride = health.URL

	orch := NewOrchestrator(db, provider, testEncKey(t), &config.Config{})

	org := createTestOrg(t, db)
	provider.registerSandbox("wake-err-ext", StatusStopped)
	sb := model.Sandbox{
		OrgID:                  &org.ID,
		ProviderID:             provider.ID(),
		ExternalID:             "wake-err-ext",
		EncryptedRuntimeSecret: []byte("enc"),
		Status:                 string(StatusStopped),
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create sandbox: %v", err)
	}

	// Short deadline so the health wait bails quickly instead of polling for the
	// full 90s employeeHealthTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	if _, err := orch.WakeSandbox(ctx, &sb); err == nil {
		t.Fatal("expected error when runtime never becomes healthy")
	}
	if sb.Status == string(StatusRunning) {
		t.Fatalf("in-memory status = running; must be error after failed health wait")
	}

	var final model.Sandbox
	if err := db.First(&final, "id = ?", sb.ID).Error; err != nil {
		t.Fatalf("reload final: %v", err)
	}
	if final.Status == string(StatusRunning) {
		t.Fatalf("persisted status = running; must not assert running on a failed wake")
	}
	if final.Status != string(StatusError) {
		t.Fatalf("persisted status = %q, want error", final.Status)
	}
}
