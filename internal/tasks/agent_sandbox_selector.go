package tasks

import (
	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/agentsandbox"
	"gorm.io/gorm"
)

func agentRuntimeSelector(db *gorm.DB, deps agentruntime.CompileDeps) agentsandbox.Selector {
	selector := agentsandbox.Selector{DB: db}
	if deps.Cfg != nil {
		selector.AgentRuntimeImage = deps.Cfg.SandboxesRuntimeBaseImage
	}
	return selector
}
