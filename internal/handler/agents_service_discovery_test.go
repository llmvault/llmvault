package handler_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestAgentHandlerEnsureServiceDiscoveryScheduleForConnection(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)
	agent := h.seedAgentAgent(t, m)
	integ := createTestIntegration(t, h.db, "railway")
	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             m.org.ID,
		UserID:            m.user.ID,
		IntegrationID:     integ.ID,
		Integration:       integ,
		NangoConnectionID: "railway-conn",
		Meta:              model.JSON{},
	}
	if err := h.db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	if err := h.handler.EnsureServiceDiscoveryScheduleForConnection(t.Context(), m.org.ID, conn); err != nil {
		t.Fatalf("EnsureServiceDiscoveryScheduleForConnection: %v", err)
	}

	var schedule model.AgentSchedule
	if err := h.db.Where("agent_id = ? AND connection_id = ?", agent.ID, conn.ID).First(&schedule).Error; err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	if !schedule.IsSystem {
		t.Fatal("schedule should be system-managed")
	}
	if schedule.Provider != "railway" {
		t.Fatalf("provider = %q, want railway", schedule.Provider)
	}
	if !strings.HasPrefix(schedule.RuntimeJobID, "system:service-discovery:railway:") {
		t.Fatalf("runtime job id = %q", schedule.RuntimeJobID)
	}
	if schedule.NextRunAt == nil {
		t.Fatal("service discovery schedule should be due immediately")
	}
	if !strings.Contains(schedule.TaskPrompt, "Railway discovery checklist") {
		t.Fatalf("schedule prompt missing Railway checklist: %s", schedule.TaskPrompt)
	}

	// The schedule must be bound to the sandbox via the *uuid.UUID SandboxID; a
	// nil/zero value means the upsert wrote a NULL association.
	if schedule.SandboxID == nil {
		t.Fatal("service discovery schedule should reference the agent sandbox")
	}
	var sandbox model.Sandbox
	if err := h.db.Where("agent_id = ?", agent.ID).First(&sandbox).Error; err != nil {
		t.Fatalf("load agent sandbox: %v", err)
	}
	if *schedule.SandboxID != sandbox.ID {
		t.Fatalf("schedule sandbox id = %s, want %s", *schedule.SandboxID, sandbox.ID)
	}

	var pushed struct {
		Schedules []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"schedules"`
	}
	if err := json.Unmarshal(h.sidecar.rawConfigBody(), &pushed); err != nil {
		t.Fatalf("decode raw config body: %v", err)
	}
	if len(pushed.Schedules) != 1 {
		t.Fatalf("pushed schedules = %d, want 1", len(pushed.Schedules))
	}
	if pushed.Schedules[0].ID != schedule.RuntimeJobID {
		t.Fatalf("pushed schedule id = %q, want %q", pushed.Schedules[0].ID, schedule.RuntimeJobID)
	}
	if pushed.Schedules[0].Source != "cron" {
		t.Fatalf("pushed schedule source = %q, want cron", pushed.Schedules[0].Source)
	}
}

func TestAgentHandlerEnsureServiceDiscoveryScheduleForGlitchTipConnection(t *testing.T) {
	h := newAgentHarness(t)
	m := h.createOrg(t)
	agent := h.seedAgentAgent(t, m)
	integ := createTestIntegration(t, h.db, "glitchtip")
	integ.DisplayName = "GlitchTip"
	conn := model.Connection{
		ID:                uuid.New(),
		OrgID:             m.org.ID,
		UserID:            m.user.ID,
		IntegrationID:     integ.ID,
		Integration:       integ,
		NangoConnectionID: "glitchtip-conn",
		Meta:              model.JSON{"connection_config": map[string]any{"baseUrl": "https://glitch.usehivy.com"}},
	}
	if err := h.db.Create(&conn).Error; err != nil {
		t.Fatalf("create connection: %v", err)
	}

	if err := h.handler.EnsureServiceDiscoveryScheduleForConnection(t.Context(), m.org.ID, conn); err != nil {
		t.Fatalf("EnsureServiceDiscoveryScheduleForConnection: %v", err)
	}

	var schedule model.AgentSchedule
	if err := h.db.Where("agent_id = ? AND connection_id = ?", agent.ID, conn.ID).First(&schedule).Error; err != nil {
		t.Fatalf("load schedule: %v", err)
	}
	if !schedule.IsSystem {
		t.Fatal("schedule should be system-managed")
	}
	if schedule.Provider != "glitchtip" {
		t.Fatalf("provider = %q, want glitchtip", schedule.Provider)
	}
	if !strings.HasPrefix(schedule.RuntimeJobID, "system:service-discovery:glitchtip:") {
		t.Fatalf("runtime job id = %q", schedule.RuntimeJobID)
	}
	if schedule.NextRunAt == nil {
		t.Fatal("service discovery schedule should be due immediately")
	}
	for _, want := range []string{
		"GlitchTip discovery checklist",
		"Load the GlitchTip skill instructions",
		"Do not resolve, delete, assign, mute, comment on, or update issues",
		"do not retain raw API payloads",
	} {
		if !strings.Contains(schedule.TaskPrompt, want) {
			t.Fatalf("schedule prompt missing %q: %s", want, schedule.TaskPrompt)
		}
	}

	var pushed struct {
		Schedules []struct {
			ID     string `json:"id"`
			Source string `json:"source"`
		} `json:"schedules"`
	}
	if err := json.Unmarshal(h.sidecar.rawConfigBody(), &pushed); err != nil {
		t.Fatalf("decode raw config body: %v", err)
	}
	if len(pushed.Schedules) != 1 {
		t.Fatalf("pushed schedules = %d, want 1", len(pushed.Schedules))
	}
	if pushed.Schedules[0].ID != schedule.RuntimeJobID {
		t.Fatalf("pushed schedule id = %q, want %q", pushed.Schedules[0].ID, schedule.RuntimeJobID)
	}
	if pushed.Schedules[0].Source != "cron" {
		t.Fatalf("pushed schedule source = %q, want cron", pushed.Schedules[0].Source)
	}
}
