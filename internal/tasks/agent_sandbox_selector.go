package tasks

import (
	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/agentsandbox"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
	"gorm.io/gorm"
)

func agentRuntimeSelector(db *gorm.DB, deps agentruntime.CompileDeps) agentsandbox.Selector {
	selector := agentsandbox.Selector{DB: db}
	if deps.Cfg != nil {
		selector.AgentRuntimeImage = sandbox.AgentRuntimeImageRef(deps.Cfg, model.SandboxImageDefault)
	}
	return selector
}
