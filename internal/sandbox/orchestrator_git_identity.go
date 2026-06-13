package sandbox

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/model"
)

type agentGitIdentity struct {
	Username string
	Email    string
}

func (o *Orchestrator) loadAgentGitIdentity(ctx context.Context, agent *model.Agent) (*agentGitIdentity, error) {
	identityAgent, err := o.resolveGitIdentityAgent(ctx, agent)
	if err != nil {
		return nil, err
	}
	if identityAgent == nil {
		return nil, nil
	}

	return gitIdentityFromProfile(identityAgent), nil
}

func (o *Orchestrator) resolveGitIdentityAgent(ctx context.Context, agent *model.Agent) (*model.Agent, error) {
	if agent == nil {
		return nil, nil
	}
	if agent.IsManaged {
		return agent, nil
	}
	if agent.OrgID == nil {
		return agent, nil
	}

	var owner model.Agent
	query := o.db.WithContext(ctx).
		Where("org_id = ? AND id <> ? AND status <> ?", *agent.OrgID, agent.ID, "archived")
	err := query.Order("created_at ASC").First(&owner).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agent, nil
		}
		return nil, err
	}
	return &owner, nil
}

func gitIdentityFromProfile(agent *model.Agent) *agentGitIdentity {
	username := fallbackGitUsername(agent)
	email := fallbackGitEmail(agent)
	return &agentGitIdentity{
		Username: strings.TrimSpace(username),
		Email:    strings.TrimSpace(email),
	}
}

func setGitIdentityEnvVars(envVars map[string]string, agent *model.Agent, identity *agentGitIdentity) {
	if agent == nil {
		return
	}
	envVars[agentruntime.AgentEnvGitUsername] = agentGitUsername(agent, identity)
	envVars[agentruntime.AgentEnvGitEmail] = agentGitEmail(agent, identity)
}

func agentGitUsername(agent *model.Agent, identity *agentGitIdentity) string {
	if identity != nil && strings.TrimSpace(identity.Username) != "" {
		return strings.TrimSpace(identity.Username)
	}
	return fallbackGitUsername(agent)
}

func agentGitEmail(agent *model.Agent, identity *agentGitIdentity) string {
	if identity != nil && strings.TrimSpace(identity.Email) != "" {
		return strings.TrimSpace(identity.Email)
	}
	return fallbackGitEmail(agent)
}

func fallbackGitUsername(agent *model.Agent) string {
	return agentruntime.AgentGitUsername(agent)
}

func fallbackGitEmail(agent *model.Agent) string {
	return agentruntime.AgentGitEmail(agent)
}
