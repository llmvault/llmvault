package tasks

import (
	"testing"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
)

func TestAgentSandboxUpgradeSkipsRuntimeDrainWhenAlwaysOnSessionsIdle(t *testing.T) {
	harness := newAgentSandboxUpgradeHarness(t)
	org, agent, channel := seedAgentSandboxUpgradeFixture(t, harness.db, "always_on")
	oldSandbox := seedAgentSandboxUpgradeSandbox(t, harness, org.ID, agent.ID, "old-runtime")
	seedAgentSandboxUpgradeSession(t, harness.db, org.ID, channel.ID, agent.ID, oldSandbox.ID, model.SessionAgentTurnIdle)
	upgrade := seedAgentSandboxUpgradeRow(t, harness.db, org.ID, agent.ID, oldSandbox.ID)

	if err := harness.handler.Handle(t.Context(), agentSandboxUpgradeTask(t, upgrade.ID, agent.ID)); err != nil {
		t.Fatalf("handle upgrade: %v", err)
	}

	harness.runtime.assertDrainCalls(t, 0, 0)
	assertAgentSandboxUpgradeSucceeded(t, harness.db, upgrade.ID)
	assertAgentSandboxUpgradeActiveSandbox(t, harness.db, org.ID, agent.ID, oldSandbox.ID)
	if len(harness.provider.stopped) != 1 || harness.provider.stopped[0] != oldSandbox.ExternalID {
		t.Fatalf("stopped=%v want old sandbox %s", harness.provider.stopped, oldSandbox.ExternalID)
	}
}

func TestAgentSandboxUpgradeDrainsWhenAnyAlwaysOnSessionActive(t *testing.T) {
	harness := newAgentSandboxUpgradeHarness(t)
	org, agent, channel := seedAgentSandboxUpgradeFixture(t, harness.db, "always_on")
	oldSandbox := seedAgentSandboxUpgradeSandbox(t, harness, org.ID, agent.ID, "old-runtime")
	seedAgentSandboxUpgradeSession(t, harness.db, org.ID, channel.ID, agent.ID, oldSandbox.ID, model.SessionAgentTurnActive)
	upgrade := seedAgentSandboxUpgradeRow(t, harness.db, org.ID, agent.ID, oldSandbox.ID)

	if err := harness.handler.Handle(t.Context(), agentSandboxUpgradeTask(t, upgrade.ID, agent.ID)); err != nil {
		t.Fatalf("handle upgrade: %v", err)
	}

	harness.runtime.assertDrainCalls(t, 1, 1)
	assertAgentSandboxUpgradeSucceeded(t, harness.db, upgrade.ID)
}

func TestAgentSandboxUpgradeContinuesWhenDrainSignalFailsTwice(t *testing.T) {
	harness := newAgentSandboxUpgradeHarness(t)
	harness.runtime.failNextDrainPOSTs(2)
	org, agent, channel := seedAgentSandboxUpgradeFixture(t, harness.db, "always_on")
	oldSandbox := seedAgentSandboxUpgradeSandbox(t, harness, org.ID, agent.ID, "old-runtime")
	seedAgentSandboxUpgradeSession(t, harness.db, org.ID, channel.ID, agent.ID, oldSandbox.ID, model.SessionAgentTurnActive)
	upgrade := seedAgentSandboxUpgradeRow(t, harness.db, org.ID, agent.ID, oldSandbox.ID)

	if err := harness.handler.Handle(t.Context(), agentSandboxUpgradeTask(t, upgrade.ID, agent.ID)); err != nil {
		t.Fatalf("handle upgrade: %v", err)
	}

	harness.runtime.assertDrainCalls(t, 2, 0)
	assertAgentSandboxUpgradeSucceeded(t, harness.db, upgrade.ID)
	assertAgentSandboxUpgradeActiveSandbox(t, harness.db, org.ID, agent.ID, oldSandbox.ID)
	if len(harness.provider.stopped) != 1 || harness.provider.stopped[0] != oldSandbox.ExternalID {
		t.Fatalf("stopped=%v want old sandbox %s", harness.provider.stopped, oldSandbox.ExternalID)
	}
}

func TestAgentSandboxUpgradeTaskMarksPerSessionAgentFailed(t *testing.T) {
	harness := newAgentSandboxUpgradeHarness(t)
	org, agent, _ := seedAgentSandboxUpgradeFixture(t, harness.db, "per_session")
	oldSandbox := seedAgentSandboxUpgradeSandbox(t, harness, org.ID, agent.ID, "old-runtime")
	upgrade := seedAgentSandboxUpgradeRow(t, harness.db, org.ID, agent.ID, oldSandbox.ID)

	if err := harness.handler.Handle(t.Context(), agentSandboxUpgradeTask(t, upgrade.ID, agent.ID)); err != nil {
		t.Fatalf("handle upgrade: %v", err)
	}

	var stored model.AgentSandboxUpgrade
	if err := harness.db.First(&stored, "id = ?", upgrade.ID).Error; err != nil {
		t.Fatalf("load upgrade: %v", err)
	}
	if stored.Status != model.AgentSandboxUpgradeStatusFailed {
		t.Fatalf("status=%s want failed", stored.Status)
	}
	if stored.ErrorMessage == nil || *stored.ErrorMessage != "per-session agents do not use sandbox upgrades" {
		t.Fatalf("error=%v", stored.ErrorMessage)
	}
	if len(harness.provider.created) != 0 {
		t.Fatalf("created replacement sandboxes=%d, want 0", len(harness.provider.created))
	}
}

func TestAgentSandboxAutoUpgradeIgnoresPerSessionAgents(t *testing.T) {
	db := connectTestDB(t)
	alwaysOrg, alwaysAgent, _ := seedAgentSandboxUpgradeFixture(t, db, "always_on")
	perOrg, perAgent, _ := seedAgentSandboxUpgradeFixture(t, db, "per_session")
	seedAgentSandboxAutoUpgradeCandidate(t, db, alwaysOrg.ID, alwaysAgent.ID, "always-on")
	seedAgentSandboxAutoUpgradeCandidate(t, db, perOrg.ID, perAgent.ID, "per-session")

	handler := NewAgentSandboxAutoUpgradeHandler(db, agentruntime.CompileDeps{Cfg: &config.Config{}}, &enqueue.MockClient{})
	rows, err := handler.loadOutdatedAgentSandboxes(t.Context(), "new-runtime-image", 100)
	if err != nil {
		t.Fatalf("load outdated sandboxes: %v", err)
	}
	if len(rows) != 1 {
		t.Fatalf("rows=%d want 1: %+v", len(rows), rows)
	}
	if rows[0].AgentID != alwaysAgent.ID {
		t.Fatalf("candidate agent=%s want always-on agent %s", rows[0].AgentID, alwaysAgent.ID)
	}
}
