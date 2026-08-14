package tasks

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

func TestSandboxCreditsUseAccumulatedVCPUMilliseconds(t *testing.T) {
	tests := []struct {
		name     string
		weighted int64
		want     int64
	}{
		{name: "fraction carries", weighted: 59_999, want: 0},
		{name: "one vcpu minute", weighted: 60_000, want: 1},
		{name: "two vcpu for thirty seconds", weighted: 2 * 30_000, want: 1},
		{name: "four vcpu for seventy seconds", weighted: 4 * 70_000, want: 4},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sandboxCreditsForWeightedMilliseconds(tt.weighted); got != tt.want {
				t.Fatalf("sandboxCreditsForWeightedMilliseconds(%d) = %d, want %d", tt.weighted, got, tt.want)
			}
		})
	}
}

func TestSyncSandboxTurnUsageSkipsDesktopProvider(t *testing.T) {
	db := connectTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "desktop-billing-" + uuid.NewString(), Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "Desktop billing"}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Desktop agent", Model: "test", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	sb := model.Sandbox{
		ID: uuid.New(), OrgID: &org.ID, AgentID: &agent.ID,
		ProviderID: sandbox.ProviderDesktop, ExternalID: "desktop-test",
		RuntimeURL: "desktop://localhost", EncryptedRuntimeSecret: []byte{1}, Status: "running",
	}
	if err := db.Create(&sb).Error; err != nil {
		t.Fatalf("create desktop sandbox: %v", err)
	}
	startedAt := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Millisecond)
	endedAt := startedAt.Add(30 * time.Second)
	session := model.Session{
		ID: uuid.New(), OrgID: org.ID, TeamID: team.ID, AgentID: agent.ID, SandboxID: &sb.ID,
		Status: "active", AgentTurnStatus: model.SessionAgentTurnIdle,
		SandboxVCPU: 1, SandboxPricingVersion: billing.SandboxPricingVersion,
		SandboxCreditsPerVCPUMinute: billing.SandboxCreditsPerVCPUMinute,
		CreatedAt:                   startedAt, UpdatedAt: endedAt,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create desktop session: %v", err)
	}
	events := []model.SessionEvent{
		{ID: uuid.New(), OrgID: org.ID, SessionID: session.ID, AgentID: agent.ID, EventID: uuid.NewString(), EventType: "turn_started", TurnID: "turn-desktop", EventAt: startedAt},
		{ID: uuid.New(), OrgID: org.ID, SessionID: session.ID, AgentID: agent.ID, EventID: uuid.NewString(), EventType: "turn_completed", TurnID: "turn-desktop", EventAt: endedAt},
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create desktop turn events: %v", err)
	}

	_, changedSessions, err := syncSandboxTurnUsage(t.Context(), db, endedAt)
	if err != nil {
		t.Fatalf("sync sandbox usage: %v", err)
	}
	if _, exists := changedSessions[session.ID]; exists {
		t.Fatalf("desktop session %s was materialized for hosted billing", session.ID)
	}
	var count int64
	if err := db.Model(&model.SandboxTurnUsage{}).Where("session_id = ?", session.ID).Count(&count).Error; err != nil {
		t.Fatalf("count desktop sandbox usage: %v", err)
	}
	if count != 0 {
		t.Fatalf("desktop sandbox usage rows = %d, want 0", count)
	}
}

func TestSandboxBillingProcessHandlerDebitsCompletedTurn(t *testing.T) {
	db := connectTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "sandbox-billing-" + uuid.NewString(), Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() { _ = db.Delete(&org).Error })
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "Billing"}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Billing agent", Model: "test", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	startedAt := time.Now().UTC().Add(-30 * time.Second).Truncate(time.Millisecond)
	endedAt := startedAt.Add(30 * time.Second)
	session := model.Session{
		ID: uuid.New(), OrgID: org.ID, TeamID: team.ID, AgentID: agent.ID,
		Status: "active", AgentTurnStatus: model.SessionAgentTurnIdle,
		SandboxVCPU: 2, SandboxPricingVersion: billing.SandboxPricingVersion,
		SandboxCreditsPerVCPUMinute: billing.SandboxCreditsPerVCPUMinute,
		CreatedAt:                   startedAt, UpdatedAt: endedAt,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	events := []model.SessionEvent{
		{ID: uuid.New(), OrgID: org.ID, SessionID: session.ID, AgentID: agent.ID, EventID: uuid.NewString(), EventType: "turn_started", TurnID: "turn-1", EventAt: startedAt},
		{ID: uuid.New(), OrgID: org.ID, SessionID: session.ID, AgentID: agent.ID, EventID: uuid.NewString(), EventType: "turn_completed", TurnID: "turn-1", EventAt: endedAt},
	}
	if err := db.Create(&events).Error; err != nil {
		t.Fatalf("create turn events: %v", err)
	}
	if err := billing.GrantWithTx(db, org.ID, 100, billing.ReasonAdjustment, "test", uuid.NewString()); err != nil {
		t.Fatalf("grant credits: %v", err)
	}

	handler := NewSandboxBillingProcessHandler(db, nil)
	if err := handler.Handle(t.Context(), nil); err != nil {
		t.Fatalf("process sandbox billing: %v", err)
	}

	var usage model.SandboxTurnUsage
	if err := db.First(&usage, "session_id = ? AND turn_id = ?", session.ID, "turn-1").Error; err != nil {
		t.Fatalf("load turn usage: %v", err)
	}
	if usage.ActiveMilliseconds != 30_000 || usage.SandboxVCPU != 2 {
		t.Fatalf("usage = %+v, want 30000ms at 2 vCPU", usage)
	}
	var debit model.CreditLedgerEntry
	if err := db.Where("org_id = ? AND reason = ?", org.ID, billing.ReasonSandboxCompute).First(&debit).Error; err != nil {
		t.Fatalf("load sandbox debit: %v", err)
	}
	if debit.Amount != -1 {
		t.Fatalf("sandbox debit = %d, want -1", debit.Amount)
	}

	if err := handler.Handle(t.Context(), nil); err != nil {
		t.Fatalf("reprocess sandbox billing: %v", err)
	}
	var count int64
	if err := db.Model(&model.CreditLedgerEntry{}).
		Where("org_id = ? AND reason = ?", org.ID, billing.ReasonSandboxCompute).Count(&count).Error; err != nil {
		t.Fatalf("count sandbox debits: %v", err)
	}
	if count != 1 {
		t.Fatalf("sandbox debit count = %d, want idempotent count 1", count)
	}
}

func TestSyncSandboxTurnUsageAdvancesActiveTurnAndClosesOnTerminalEvent(t *testing.T) {
	db := connectTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "sandbox-active-" + uuid.NewString(), Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	team := model.Team{ID: uuid.New(), OrgID: org.ID, Name: "Active billing"}
	if err := db.Create(&team).Error; err != nil {
		t.Fatalf("create team: %v", err)
	}
	agent := model.Agent{ID: uuid.New(), OrgID: &org.ID, TeamID: team.ID, Name: "Active agent", Model: "test", Status: "active"}
	if err := db.Create(&agent).Error; err != nil {
		t.Fatalf("create agent: %v", err)
	}
	startedAt := time.Now().UTC().Add(-time.Minute).Truncate(time.Millisecond)
	session := model.Session{
		ID: uuid.New(), OrgID: org.ID, TeamID: team.ID, AgentID: agent.ID,
		Status: "active", AgentTurnStatus: model.SessionAgentTurnActive, AgentTurnID: "turn-active",
		SandboxVCPU: 4, SandboxPricingVersion: billing.SandboxPricingVersion,
		SandboxCreditsPerVCPUMinute: billing.SandboxCreditsPerVCPUMinute,
		CreatedAt:                   startedAt, UpdatedAt: startedAt,
	}
	if err := db.Create(&session).Error; err != nil {
		t.Fatalf("create session: %v", err)
	}
	startEvent := model.SessionEvent{
		ID: uuid.New(), OrgID: org.ID, SessionID: session.ID, AgentID: agent.ID,
		EventID: uuid.NewString(), EventType: "turn_started", TurnID: "turn-active", EventAt: startedAt,
	}
	if err := db.Create(&startEvent).Error; err != nil {
		t.Fatalf("create start event: %v", err)
	}

	activeThrough := startedAt.Add(25 * time.Second)
	_, changedSessions, err := syncSandboxTurnUsage(t.Context(), db, activeThrough)
	if err != nil {
		t.Fatalf("sync active turn: %v", err)
	}
	if changedSessions[session.ID] != org.ID {
		t.Fatalf("changed sessions = %v, want session %s for org %s", changedSessions, session.ID, org.ID)
	}
	var usage model.SandboxTurnUsage
	if err := db.First(&usage, "session_id = ? AND turn_id = ?", session.ID, "turn-active").Error; err != nil {
		t.Fatalf("load active usage: %v", err)
	}
	if usage.ActiveMilliseconds != 25_000 || usage.EndedAt != nil {
		t.Fatalf("active usage = %+v, want 25000ms and no end", usage)
	}

	endedAt := startedAt.Add(40 * time.Second)
	terminal := model.SessionEvent{
		ID: uuid.New(), OrgID: org.ID, SessionID: session.ID, AgentID: agent.ID,
		EventID: uuid.NewString(), EventType: "turn_completed", TurnID: "turn-active", EventAt: endedAt,
	}
	if err := db.Create(&terminal).Error; err != nil {
		t.Fatalf("create terminal event: %v", err)
	}
	if err := db.Model(&model.Session{}).Where("id = ? AND org_id = ?", session.ID, org.ID).
		Updates(map[string]any{"agent_turn_status": model.SessionAgentTurnIdle, "agent_turn_id": "", "updated_at": endedAt}).Error; err != nil {
		t.Fatalf("close session turn: %v", err)
	}
	_, changedSessions, err = syncSandboxTurnUsage(t.Context(), db, endedAt.Add(time.Minute))
	if err != nil {
		t.Fatalf("sync completed turn: %v", err)
	}
	if changedSessions[session.ID] != org.ID {
		t.Fatalf("completed changed sessions = %v, want session %s for org %s", changedSessions, session.ID, org.ID)
	}
	if err := db.First(&usage, "session_id = ? AND turn_id = ?", session.ID, "turn-active").Error; err != nil {
		t.Fatalf("reload completed usage: %v", err)
	}
	if usage.ActiveMilliseconds != 40_000 || usage.EndedAt == nil || !usage.EndedAt.Equal(endedAt) {
		t.Fatalf("completed usage = %+v, want 40000ms ending at %s", usage, endedAt)
	}
}
