package agentruntime

import (
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

func agentWithRuntimeModel(agent *model.Agent, modelID string) *model.Agent {
	modelID = strings.TrimSpace(modelID)
	if modelID == "" || agent == nil {
		return agent
	}
	runtimeAgent := *agent
	runtimeAgent.Model = modelID
	return &runtimeAgent
}
