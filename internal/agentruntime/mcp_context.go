package agentruntime

import (
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

const (
	MCPInvocationWeb      = "web"
	MCPInvocationSchedule = "schedule"
	MCPInvocationCron     = "cron"
)

// MCPRuntimeContext identifies the human whose personal MCP servers may be
// compiled into a runtime definition. Source is an explicit security boundary:
// Slack, webhook, automation, and unknown sources fail closed even when a row
// happens to carry a user id.
type MCPRuntimeContext struct {
	ActorUserID *uuid.UUID
	Source      string
}

func (c MCPRuntimeContext) AllowsPersonalServers() bool {
	if c.ActorUserID == nil || *c.ActorUserID == uuid.Nil {
		return false
	}
	switch strings.ToLower(strings.TrimSpace(c.Source)) {
	case MCPInvocationWeb, MCPInvocationSchedule, MCPInvocationCron:
		return true
	default:
		return false
	}
}

// MCPRuntimeContextForSession derives the actor-aware MCP context for a turn.
// A web turn uses its authenticated sender; a schedule/cron run uses the user
// persisted on the schedule-created session. External sources never receive
// personal MCP servers because their upstream identity is not yet mapped to a
// Hivy user.
func MCPRuntimeContextForSession(session model.Session, turnActor *uuid.UUID) MCPRuntimeContext {
	source := strings.ToLower(strings.TrimSpace(session.Source))
	switch source {
	case model.SessionSourceWeb:
		actor := turnActor
		if actor == nil {
			actor = session.CreatedBy
		}
		return MCPRuntimeContext{ActorUserID: actor, Source: MCPInvocationWeb}
	case MCPInvocationSchedule, MCPInvocationCron:
		return MCPRuntimeContext{ActorUserID: session.CreatedBy, Source: source}
	default:
		return MCPRuntimeContext{Source: source}
	}
}
