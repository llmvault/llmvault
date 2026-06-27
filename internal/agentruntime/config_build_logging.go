package agentruntime

import (
	"context"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type runtimeConfigBuildPhaseLogger struct {
	ctx          context.Context
	agent        *model.Agent
	sb           *model.Sandbox
	totalStarted time.Time
	phaseStarted time.Time
}

func newRuntimeConfigBuildPhaseLogger(ctx context.Context, agent *model.Agent, sb *model.Sandbox) *runtimeConfigBuildPhaseLogger {
	started := time.Now()
	return &runtimeConfigBuildPhaseLogger{ctx: ctx, agent: agent, sb: sb, totalStarted: started, phaseStarted: started}
}

func (l *runtimeConfigBuildPhaseLogger) log(phase string, attrs ...any) {
	if l.agent != nil {
		attrs = append(attrs, "agent_id", l.agent.ID)
		if l.agent.OrgID != nil {
			attrs = append(attrs, "org_id", *l.agent.OrgID)
		}
	}
	if l.sb != nil {
		attrs = append(attrs, "sandbox_id", l.sb.ID)
	}
	attrs = append(attrs, "total_ms", time.Since(l.totalStarted).Milliseconds())
	logging.LogPhase(l.ctx, "agent runtime config build phase", phase, l.phaseStarted, attrs...)
	l.phaseStarted = time.Now()
}
