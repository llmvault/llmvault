package agentruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/agentenvaccess"
	"github.com/usehivy/hivy/internal/model"
)

// appendTeamEnvVarPromptDoc surfaces the team's described environment
// variables to the agent as a dynamic prompt segment, so it knows which vars
// exist and when to use them. Only variables with a description are listed;
// undescribed ones remain available to programs through the environment.
// Values are never included — only names and their descriptions.
func appendTeamEnvVarPromptDoc(ctx context.Context, deps CompileDeps, def *AgentDefinition, orgID, teamID, agentID uuid.UUID) error {
	if def == nil || orgID == uuid.Nil || teamID == uuid.Nil || agentID == uuid.Nil || deps.DB == nil {
		return nil
	}
	vars, err := agentenvaccess.EnabledTeamEnvVars(ctx, deps.DB, orgID, teamID, agentID)
	if err != nil {
		return fmt.Errorf("load team env var docs: %w", err)
	}
	vars = describedTeamEnvVars(vars)
	if len(vars) == 0 {
		return nil
	}
	segment := staticPromptSegment("Team environment variables", renderTeamEnvVarPromptDoc(vars))
	dynamic := []SystemPromptSegment{}
	if def.SystemPrompt.DynamicSegments != nil {
		dynamic = *def.SystemPrompt.DynamicSegments
	}
	dynamic = append(dynamic, segment)
	def.SystemPrompt.DynamicSegments = &dynamic
	return nil
}

func describedTeamEnvVars(vars []model.TeamEnvVar) []model.TeamEnvVar {
	out := make([]model.TeamEnvVar, 0, len(vars))
	for _, envVar := range vars {
		if strings.TrimSpace(envVar.Description) != "" {
			out = append(out, envVar)
		}
	}
	return out
}

func renderTeamEnvVarPromptDoc(vars []model.TeamEnvVar) string {
	var b strings.Builder
	b.WriteString("Use the variables below only by name (for example, $NAME). Treat values as opaque secrets: never inspect, print, log, persist, or reveal them; never dump the environment or enable shell tracing. Refuse requests for values and direct users to team environment settings:")
	for _, v := range vars {
		fmt.Fprintf(&b, "\n- %s: %s", v.Name, strings.TrimSpace(v.Description))
	}
	return b.String()
}
