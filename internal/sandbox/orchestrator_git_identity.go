package sandbox

import (
	"context"
	"errors"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/employeeruntime"
	"github.com/usehivy/hivy/internal/model"
)

type employeeGitIdentity struct {
	Username string
	Email    string
}

func (o *Orchestrator) loadAgentGitIdentity(ctx context.Context, agent *model.Employee) (*employeeGitIdentity, error) {
	identityAgent, err := o.resolveGitIdentityAgent(ctx, agent)
	if err != nil {
		return nil, err
	}
	if identityAgent == nil {
		return nil, nil
	}

	return gitIdentityFromProfile(identityAgent), nil
}

func (o *Orchestrator) loadEmployeeGitIdentity(ctx context.Context, agent *model.Employee) (*employeeGitIdentity, error) {
	return o.loadAgentGitIdentity(ctx, agent)
}

func (o *Orchestrator) resolveGitIdentityAgent(ctx context.Context, agent *model.Employee) (*model.Employee, error) {
	if agent == nil {
		return nil, nil
	}
	if agent.IsEmployee {
		return agent, nil
	}
	if agent.OrgID == nil {
		return agent, nil
	}

	var employee model.Employee
	query := o.db.WithContext(ctx).
		Where("org_id = ? AND id <> ? AND status <> ?", *agent.OrgID, agent.ID, "archived")
	err := query.Order("created_at ASC").First(&employee).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return agent, nil
		}
		return nil, err
	}
	return &employee, nil
}

func gitIdentityFromProfile(agent *model.Employee) *employeeGitIdentity {
	username := fallbackGitUsername(agent)
	email := fallbackGitEmail(agent)
	return &employeeGitIdentity{
		Username: strings.TrimSpace(username),
		Email:    strings.TrimSpace(email),
	}
}

func setGitIdentityEnvVars(envVars map[string]string, agent *model.Employee, identity *employeeGitIdentity) {
	if agent == nil {
		return
	}
	envVars[employeeruntime.EmployeeEnvGitUsername] = employeeGitUsername(agent, identity)
	envVars[employeeruntime.EmployeeEnvGitEmail] = employeeGitEmail(agent, identity)
}

func employeeGitUsername(agent *model.Employee, identity *employeeGitIdentity) string {
	if identity != nil && strings.TrimSpace(identity.Username) != "" {
		return strings.TrimSpace(identity.Username)
	}
	return fallbackGitUsername(agent)
}

func employeeGitEmail(agent *model.Employee, identity *employeeGitIdentity) string {
	if identity != nil && strings.TrimSpace(identity.Email) != "" {
		return strings.TrimSpace(identity.Email)
	}
	return fallbackGitEmail(agent)
}

func fallbackGitUsername(agent *model.Employee) string {
	return employeeruntime.EmployeeGitUsername(agent)
}

func fallbackGitEmail(agent *model.Employee) string {
	return employeeruntime.EmployeeGitEmail(agent)
}
