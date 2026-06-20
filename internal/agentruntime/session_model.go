package agentruntime

import (
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

// SessionModelDefinition builds the per-message model override sent with a
// session message. It must stay side-effect free: hot message delivery should
// not push runtime config or mint session-scoped runtime env.
func SessionModelDefinition(
	deps CompileDeps,
	agent *model.Agent,
	modelID string,
	reasoningEffort string,
) (*ModelConfig, error) {
	if agent == nil {
		return nil, fmt.Errorf("session model definition: agent is required")
	}
	modelID = strings.TrimSpace(modelID)
	if modelID == "" {
		modelID = strings.TrimSpace(agent.Model)
	}
	if modelID == "" {
		modelID = DefaultAgentModel
	}
	sessionModel := ProxyModelConfig(deps.Cfg, modelID, reasoningEffort)
	sessionModel.APIKeyEnv = ProxyAPIKeyEnv
	return &sessionModel, nil
}
