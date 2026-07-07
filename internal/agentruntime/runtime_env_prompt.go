package agentruntime

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// appendChannelEnvVarPromptDoc surfaces the channel's described environment
// variables to the agent as a dynamic prompt segment, so it knows which vars
// exist and when to use them. Only variables with a description are listed;
// undescribed ones stay discoverable only from the shell (unchanged behavior).
// Values are never included — only names and their descriptions.
func appendChannelEnvVarPromptDoc(ctx context.Context, deps CompileDeps, def *AgentDefinition, channelID uuid.UUID) error {
	if def == nil || channelID == uuid.Nil || deps.DB == nil {
		return nil
	}
	var vars []model.ChannelEnvVar
	if err := deps.DB.WithContext(ctx).
		Select("name", "description").
		Where("channel_id = ? AND description <> ''", channelID).
		Order("name").
		Find(&vars).Error; err != nil {
		return fmt.Errorf("load channel env var docs: %w", err)
	}
	if len(vars) == 0 {
		return nil
	}
	var b strings.Builder
	b.WriteString("These environment variables are set in this channel's sessions. Read them from the shell environment (e.g. $NAME) when the described situation applies:")
	for _, v := range vars {
		fmt.Fprintf(&b, "\n- %s: %s", v.Name, strings.TrimSpace(v.Description))
	}
	segment := staticPromptSegment("Channel environment variables", b.String())
	dynamic := []SystemPromptSegment{}
	if def.SystemPrompt.DynamicSegments != nil {
		dynamic = *def.SystemPrompt.DynamicSegments
	}
	dynamic = append(dynamic, segment)
	def.SystemPrompt.DynamicSegments = &dynamic
	return nil
}
