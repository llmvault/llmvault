package agentruntime

import (
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestMCPRuntimeContextForSessionPersonalServerPolicy(t *testing.T) {
	creator := uuid.New()
	turnActor := uuid.New()
	tests := []struct {
		name       string
		session    model.Session
		turnActor  *uuid.UUID
		wantActor  *uuid.UUID
		wantSource string
		wantAllow  bool
	}{
		{name: "web uses authenticated turn actor", session: model.Session{Source: model.SessionSourceWeb, CreatedBy: &creator}, turnActor: &turnActor, wantActor: &turnActor, wantSource: MCPInvocationWeb, wantAllow: true},
		{name: "web falls back to session creator", session: model.Session{Source: model.SessionSourceWeb, CreatedBy: &creator}, wantActor: &creator, wantSource: MCPInvocationWeb, wantAllow: true},
		{name: "schedule uses persisted creator", session: model.Session{Source: MCPInvocationSchedule, CreatedBy: &creator}, wantActor: &creator, wantSource: MCPInvocationSchedule, wantAllow: true},
		{name: "cron uses persisted creator", session: model.Session{Source: MCPInvocationCron, CreatedBy: &creator}, wantActor: &creator, wantSource: MCPInvocationCron, wantAllow: true},
		{name: "slack external fails closed", session: model.Session{Source: model.SessionSourceExternal, CreatedBy: &creator}, turnActor: &turnActor, wantSource: model.SessionSourceExternal, wantAllow: false},
		{name: "automation fails closed", session: model.Session{Source: "automation", CreatedBy: &creator}, turnActor: &turnActor, wantSource: "automation", wantAllow: false},
		{name: "webhook fails closed", session: model.Session{Source: "webhook", CreatedBy: &creator}, turnActor: &turnActor, wantSource: "webhook", wantAllow: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := MCPRuntimeContextForSession(tt.session, tt.turnActor)
			if got.Source != tt.wantSource {
				t.Fatalf("source = %q, want %q", got.Source, tt.wantSource)
			}
			if !sameOptionalUUID(got.ActorUserID, tt.wantActor) {
				t.Fatalf("actor = %v, want %v", got.ActorUserID, tt.wantActor)
			}
			if got.AllowsPersonalServers() != tt.wantAllow {
				t.Fatalf("AllowsPersonalServers = %v, want %v", got.AllowsPersonalServers(), tt.wantAllow)
			}
		})
	}
}

func sameOptionalUUID(left, right *uuid.UUID) bool {
	if left == nil || right == nil {
		return left == nil && right == nil
	}
	return *left == *right
}
