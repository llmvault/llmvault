package tasks

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmployeeProxyTokenRefreshHandler_InjectsNewTokenRevokesOldAndSchedulesNext(t *testing.T) {
	f := newEmployeeProxyTokenRefreshFixture(t, 0)
	oldToken, err := agentruntime.MintProxyToken(context.Background(), f.compileDeps, &f.agent, f.sandbox.ID)
	if err != nil {
		t.Fatalf("mint old proxy token: %v", err)
	}
	// Backdate the launch token past the revoke grace window so it is eligible for
	// revocation (the refresh only protects tokens within the grace window).
	if err := f.db.Model(&model.Token{}).Where("jti = ?", oldToken.JTI).
		Update("created_at", time.Now().Add(-2*employeeProxyTokenRevokeGrace)).Error; err != nil {
		t.Fatalf("backdate old token: %v", err)
	}

	task, _, err := NewEmployeeProxyTokenRefreshTask(EmployeeProxyTokenRefreshPayload{
		EmployeeID:  f.agent.ID,
		SandboxID:   f.sandbox.ID,
		ScheduledAt: oldToken.ExpiresAt.Add(-employeeProxyTokenRefreshLead),
	})
	if err != nil {
		t.Fatalf("new refresh task: %v", err)
	}
	if err := f.handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle refresh: %v", err)
	}

	f.runtime.mu.Lock()
	envCalls := f.runtime.envCalls
	injected := f.runtime.lastEnv[agentruntime.ProxyAPIKeyEnv]
	runtimeSecret := f.runtime.lastEnv[agentruntime.EmployeeEnvRuntimeSecret]
	employeeID := f.runtime.lastEnv[agentruntime.EmployeeEnvEmployeeID]
	f.runtime.mu.Unlock()
	if envCalls != 1 {
		t.Fatalf("runtime env calls = %d, want 1", envCalls)
	}
	if !strings.HasPrefix(injected, "ptok_") || injected == oldToken.Token {
		t.Fatalf("injected proxy token was not refreshed: %q", injected)
	}
	if runtimeSecret == "" {
		t.Fatalf("runtime config did not include runtime secret env")
	}
	if employeeID != f.agent.ID.String() {
		t.Fatalf("runtime config employee id env = %q, want %s", employeeID, f.agent.ID)
	}

	var old model.Token
	if err := f.db.First(&old, "jti = ?", oldToken.JTI).Error; err != nil {
		t.Fatalf("load old token: %v", err)
	}
	if old.RevokedAt == nil {
		t.Fatal("old token was not revoked")
	}
	var activeCount int64
	if err := f.db.Model(&model.Token{}).
		Where("org_id = ? AND meta->>? = ? AND meta->>? = ? AND revoked_at IS NULL",
			f.org.ID,
			model.TokenMetaAgentID, f.agent.ID.String(),
			model.TokenMetaHarness, model.TokenHarnessAgentSandbox).
		Count(&activeCount).Error; err != nil {
		t.Fatalf("count active tokens: %v", err)
	}
	if activeCount != 1 {
		t.Fatalf("active employee proxy tokens = %d, want 1", activeCount)
	}
	var refreshedAgent model.Employee
	if err := f.db.First(&refreshedAgent, "id = ?", f.agent.ID).Error; err != nil {
		t.Fatalf("load refreshed agent: %v", err)
	}
	if refreshedAgent.LastProxyTokenRefreshedAt == nil {
		t.Fatal("last_proxy_token_refreshed_at was not set")
	}

	_ = requireProxyRefreshTask(t, f.enqueuer)
}

// A proxy token minted by a concurrent sync (within the grace window) must survive the refresh's
// revoke-older sweep, or the runtime authenticates with a revoked token.
func TestEmployeeProxyTokenRefreshHandler_DoesNotRevokeConcurrentlyMintedToken(t *testing.T) {
	f := newEmployeeProxyTokenRefreshFixture(t, 0)

	// The token currently in use, minted long ago at launch.
	oldToken, err := agentruntime.MintProxyToken(context.Background(), f.compileDeps, &f.agent, f.sandbox.ID)
	if err != nil {
		t.Fatalf("mint old proxy token: %v", err)
	}
	if err := f.db.Model(&model.Token{}).Where("jti = ?", oldToken.JTI).
		Update("created_at", time.Now().Add(-2*employeeProxyTokenRevokeGrace)).Error; err != nil {
		t.Fatalf("backdate old token: %v", err)
	}

	// A concurrent sync mints a fresh token just before the refresh runs.
	concurrentToken, err := agentruntime.MintProxyToken(context.Background(), f.compileDeps, &f.agent, f.sandbox.ID)
	if err != nil {
		t.Fatalf("mint concurrent proxy token: %v", err)
	}

	task, _, err := NewEmployeeProxyTokenRefreshTask(EmployeeProxyTokenRefreshPayload{
		EmployeeID:  f.agent.ID,
		SandboxID:   f.sandbox.ID,
		ScheduledAt: oldToken.ExpiresAt.Add(-employeeProxyTokenRefreshLead),
	})
	if err != nil {
		t.Fatalf("new refresh task: %v", err)
	}
	if err := f.handler.Handle(context.Background(), task); err != nil {
		t.Fatalf("handle refresh: %v", err)
	}

	var concurrent model.Token
	if err := f.db.First(&concurrent, "jti = ?", concurrentToken.JTI).Error; err != nil {
		t.Fatalf("load concurrent token: %v", err)
	}
	if concurrent.RevokedAt != nil {
		t.Fatal("concurrently-minted token within the grace window was revoked; grace window must protect it")
	}

	var old model.Token
	if err := f.db.First(&old, "jti = ?", oldToken.JTI).Error; err != nil {
		t.Fatalf("load old token: %v", err)
	}
	if old.RevokedAt == nil {
		t.Fatal("old (pre-grace) token should still be revoked")
	}
}

func TestEmployeeProxyTokenRefreshHandler_RevokesMintedTokenWhenRuntimeRejectsEnv(t *testing.T) {
	f := newEmployeeProxyTokenRefreshFixture(t, http.StatusInternalServerError)
	if _, err := agentruntime.MintProxyToken(context.Background(), f.compileDeps, &f.agent, f.sandbox.ID); err != nil {
		t.Fatalf("mint old proxy token: %v", err)
	}
	task, _, err := NewEmployeeProxyTokenRefreshTask(EmployeeProxyTokenRefreshPayload{
		EmployeeID: f.agent.ID,
		SandboxID:  f.sandbox.ID,
	})
	if err != nil {
		t.Fatalf("new refresh task: %v", err)
	}
	if err := f.handler.Handle(context.Background(), task); err == nil {
		t.Fatal("expected runtime env failure")
	}

	var revokedCount int64
	if err := f.db.Model(&model.Token{}).
		Where("org_id = ? AND meta->>? = ? AND meta->>? = ? AND revoked_at IS NOT NULL",
			f.org.ID,
			model.TokenMetaAgentID, f.agent.ID.String(),
			model.TokenMetaHarness, model.TokenHarnessAgentSandbox).
		Count(&revokedCount).Error; err != nil {
		t.Fatalf("count revoked tokens: %v", err)
	}
	if revokedCount == 0 {
		t.Fatal("failed refresh token was not revoked")
	}
	if len(f.enqueuer.Tasks()) != 0 {
		t.Fatalf("next refresh should not be scheduled after failure: %v", f.enqueuer.Tasks())
	}
}

func TestNextEmployeeProxyTokenRefreshAt_UsesActiveStartupTokenExpiry(t *testing.T) {
	f := newEmployeeProxyTokenRefreshFixture(t, 0)
	startupToken, err := agentruntime.MintProxyToken(context.Background(), f.compileDeps, &f.agent, f.sandbox.ID)
	if err != nil {
		t.Fatalf("mint startup proxy token: %v", err)
	}
	now := startupToken.ExpiresAt.Add(-12 * employeeProxyTokenRefreshLead)

	got, err := nextEmployeeProxyTokenRefreshAt(context.Background(), f.db, &f.agent, f.sandbox.ID, now)
	if err != nil {
		t.Fatalf("next refresh: %v", err)
	}
	want := startupToken.ExpiresAt.Add(-employeeProxyTokenRefreshLead).UTC().Truncate(time.Microsecond)
	if !got.Equal(want) {
		t.Fatalf("next refresh = %s, want %s", got, want)
	}
}
