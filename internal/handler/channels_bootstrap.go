package handler

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// provisionTeamDefaults makes a freshly created team self-sufficient by cloning
// the built-in Hivy into that team. The agent is the team's default Hivy; teams
// no longer create a synthetic #general channel.
func provisionTeamDefaults(ctx context.Context, tx *gorm.DB, orgID, teamID, createdByUserID uuid.UUID) (*model.Agent, error) {
	_ = createdByUserID
	agent, err := createHivyAgentWithDefaultsTx(ctx, tx, orgID, teamID)
	if err != nil {
		return nil, fmt.Errorf("provision team Hivy agent: %w", err)
	}
	return agent, nil
}
