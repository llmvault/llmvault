// Package agentenvaccess resolves and updates per-agent access to environment
// variables inherited from the agent's team.
package agentenvaccess

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

var (
	ErrAgentNotFound               = errors.New("agent environment access: agent not found")
	ErrEnvironmentVariableNotFound = errors.New("agent environment access: environment variable not found")
)

type Variable struct {
	Name        string
	Description string
	Enabled     bool
}

func LoadAgent(ctx context.Context, db *gorm.DB, orgID, agentID uuid.UUID) (model.Agent, error) {
	var agent model.Agent
	err := db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ? AND parent_agent_id IS NULL", agentID, orgID, "archived").
		First(&agent).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.Agent{}, ErrAgentNotFound
	}
	if err != nil {
		return model.Agent{}, fmt.Errorf("load agent: %w", err)
	}
	return agent, nil
}

func List(ctx context.Context, db *gorm.DB, agent model.Agent) ([]Variable, error) {
	if agent.OrgID == nil || *agent.OrgID == uuid.Nil || agent.TeamID == uuid.Nil {
		return []Variable{}, nil
	}
	type variableRow struct {
		Name        string
		Description string
		Enabled     bool
	}
	var rows []variableRow
	if err := db.WithContext(ctx).
		Table("team_env_vars AS env_vars").
		Select("env_vars.name, env_vars.description, (denies.agent_id IS NULL) AS enabled").
		Joins(
			"LEFT JOIN agent_team_env_var_denies AS denies ON denies.team_env_var_id = env_vars.id AND denies.agent_id = ? AND denies.org_id = env_vars.org_id",
			agent.ID,
		).
		Where("env_vars.org_id = ? AND env_vars.team_id = ?", *agent.OrgID, agent.TeamID).
		Order("env_vars.name").
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list team environment variables: %w", err)
	}
	out := make([]Variable, len(rows))
	for i, row := range rows {
		out[i] = Variable(row)
	}
	return out, nil
}

func SetEnabled(ctx context.Context, db *gorm.DB, agent model.Agent, name string, enabled bool) (Variable, error) {
	if agent.OrgID == nil || *agent.OrgID == uuid.Nil || agent.TeamID == uuid.Nil {
		return Variable{}, ErrEnvironmentVariableNotFound
	}
	var envVar model.TeamEnvVar
	err := db.WithContext(ctx).
		Select("id", "org_id", "team_id", "name", "description").
		Where("org_id = ? AND team_id = ? AND name = ?", *agent.OrgID, agent.TeamID, name).
		First(&envVar).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return Variable{}, ErrEnvironmentVariableNotFound
	}
	if err != nil {
		return Variable{}, fmt.Errorf("load team environment variable: %w", err)
	}

	deny := model.AgentTeamEnvVarDeny{
		OrgID:        *agent.OrgID,
		AgentID:      agent.ID,
		TeamEnvVarID: envVar.ID,
	}
	if enabled {
		if err := db.WithContext(ctx).
			Where("org_id = ? AND agent_id = ? AND team_env_var_id = ?", deny.OrgID, deny.AgentID, deny.TeamEnvVarID).
			Delete(&model.AgentTeamEnvVarDeny{}).Error; err != nil {
			return Variable{}, fmt.Errorf("enable team environment variable: %w", err)
		}
	} else if err := db.WithContext(ctx).
		Clauses(clause.OnConflict{DoNothing: true}).
		Create(&deny).Error; err != nil {
		return Variable{}, fmt.Errorf("disable team environment variable: %w", err)
	}
	return Variable{Name: envVar.Name, Description: envVar.Description, Enabled: enabled}, nil
}

// EnabledTeamEnvVars returns only variables the agent still inherits. Runtime
// environment and prompt assembly both use this query so a disabled secret is
// neither injected nor described to the agent.
func EnabledTeamEnvVars(ctx context.Context, db *gorm.DB, orgID, teamID, agentID uuid.UUID) ([]model.TeamEnvVar, error) {
	if db == nil || orgID == uuid.Nil || teamID == uuid.Nil || agentID == uuid.Nil {
		return []model.TeamEnvVar{}, nil
	}
	var vars []model.TeamEnvVar
	if err := db.WithContext(ctx).
		Table("team_env_vars AS env_vars").
		Select("env_vars.*").
		Joins(
			"LEFT JOIN agent_team_env_var_denies AS denies ON denies.team_env_var_id = env_vars.id AND denies.agent_id = ? AND denies.org_id = env_vars.org_id",
			agentID,
		).
		Where("env_vars.org_id = ? AND env_vars.team_id = ? AND denies.agent_id IS NULL", orgID, teamID).
		Order("env_vars.name").
		Find(&vars).Error; err != nil {
		return nil, fmt.Errorf("load enabled team environment variables: %w", err)
	}
	return vars, nil
}
