package agentrouter

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// RecentChannelSessions loads the channel's most recently updated sessions
// across all agents, newest first, for topical-continuity context. It returns
// at most RecentSessionsLimit rows. Sessions without a resolvable agent name are
// skipped rather than failing the query.
func RecentChannelSessions(ctx context.Context, db *gorm.DB, orgID, channelID uuid.UUID) ([]RecentSession, error) {
	if db == nil {
		return nil, nil
	}

	type row struct {
		Name      string
		AgentName string
	}
	var rows []row
	err := db.WithContext(ctx).
		Table("sessions").
		Select("sessions.name AS name, agents.name AS agent_name").
		Joins("LEFT JOIN agents ON agents.id = sessions.agent_id").
		Where("sessions.org_id = ? AND sessions.channel_id = ?", orgID, channelID).
		Order("sessions.updated_at DESC").
		Limit(RecentSessionsLimit).
		Scan(&rows).Error
	if err != nil {
		return nil, fmt.Errorf("load recent channel sessions: %w", err)
	}

	sessions := make([]RecentSession, 0, len(rows))
	for _, r := range rows {
		sessions = append(sessions, RecentSession{Name: r.Name, AgentName: r.AgentName})
	}
	return sessions, nil
}

// CandidatesFromAgents adapts assigned agent models into router candidates,
// using the agent's description as its role, falling back to its instructions.
func CandidatesFromAgents(agents []model.Agent) []Candidate {
	candidates := make([]Candidate, 0, len(agents))
	for i := range agents {
		candidates = append(candidates, Candidate{
			ID:   agents[i].ID,
			Name: agents[i].Name,
			Role: agentRole(agents[i]),
		})
	}
	return candidates
}

func agentRole(agent model.Agent) string {
	if agent.Description != nil && *agent.Description != "" {
		return *agent.Description
	}
	if agent.Instructions != nil {
		return *agent.Instructions
	}
	return ""
}
