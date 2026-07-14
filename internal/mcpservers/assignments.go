package mcpservers

import (
	"context"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func (s *Service) GrantTeam(ctx context.Context, orgID, teamID, serverID uuid.UUID, grantedBy *uuid.UUID) error {
	if ok, err := s.teamExists(ctx, orgID, teamID); err != nil {
		return err
	} else if !ok {
		return ErrTeamNotFound
	}
	server, err := s.GetServer(ctx, orgID, serverID, nil)
	if err != nil {
		return err
	}
	if server.Scope != model.MCPServerScopeOrg {
		return ErrNotFound
	}
	row := model.TeamMCPServer{OrgID: orgID, TeamID: teamID, MCPServerID: serverID, GrantedBy: grantedBy}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "team_id"}, {Name: "mcp_server_id"}}, DoNothing: true,
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("grant team mcp server: %w", err)
	}
	return nil
}

func (s *Service) RevokeTeam(ctx context.Context, orgID, teamID, serverID uuid.UUID) error {
	if ok, err := s.teamExists(ctx, orgID, teamID); err != nil {
		return err
	} else if !ok {
		return ErrTeamNotFound
	}
	if _, err := s.GetServer(ctx, orgID, serverID, nil); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Where("org_id = ? AND team_id = ? AND mcp_server_id = ?", orgID, teamID, serverID).
		Delete(&model.TeamMCPServer{}).Error; err != nil {
		return fmt.Errorf("revoke team mcp server: %w", err)
	}
	return nil
}

func (s *Service) TeamServers(ctx context.Context, orgID, teamID uuid.UUID) ([]model.MCPServer, error) {
	if ok, err := s.teamExists(ctx, orgID, teamID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrTeamNotFound
	}
	servers := []model.MCPServer{}
	if err := s.db.WithContext(ctx).Table("mcp_servers ms").Select("ms.*").
		Joins("JOIN team_mcp_servers tms ON tms.mcp_server_id = ms.id AND tms.org_id = ms.org_id").
		Where("ms.org_id = ? AND tms.team_id = ? AND ms.scope = ?", orgID, teamID, model.MCPServerScopeOrg).
		Order("ms.name ASC, ms.id ASC").Scan(&servers).Error; err != nil {
		return nil, fmt.Errorf("list team mcp servers: %w", err)
	}
	return servers, nil
}

func (s *Service) SetAgentGrant(ctx context.Context, orgID, agentID, serverID uuid.UUID, state string, updatedBy *uuid.UUID) error {
	if state != model.MCPAgentGrantEnabled && state != model.MCPAgentGrantDisabled {
		return validationErrorf("state must be enabled or disabled")
	}
	if ok, err := s.agentExists(ctx, orgID, agentID); err != nil {
		return err
	} else if !ok {
		return ErrAgentNotFound
	}
	server, err := s.GetServer(ctx, orgID, serverID, nil)
	if err != nil {
		return err
	}
	if server.Scope != model.MCPServerScopeOrg {
		return ErrNotFound
	}
	row := model.AgentMCPServer{OrgID: orgID, AgentID: agentID, MCPServerID: serverID, State: state, UpdatedBy: updatedBy}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "agent_id"}, {Name: "mcp_server_id"}},
		DoUpdates: clause.Assignments(map[string]any{"state": state, "updated_by": updatedBy}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("set agent mcp server grant: %w", err)
	}
	return nil
}

func (s *Service) DeleteAgentGrant(ctx context.Context, orgID, agentID, serverID uuid.UUID) error {
	if ok, err := s.agentExists(ctx, orgID, agentID); err != nil {
		return err
	} else if !ok {
		return ErrAgentNotFound
	}
	if _, err := s.GetServer(ctx, orgID, serverID, nil); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Where("org_id = ? AND agent_id = ? AND mcp_server_id = ?", orgID, agentID, serverID).
		Delete(&model.AgentMCPServer{}).Error; err != nil {
		return fmt.Errorf("delete agent mcp server grant: %w", err)
	}
	return nil
}

func (s *Service) AgentGrants(ctx context.Context, orgID, agentID uuid.UUID) ([]AgentGrant, error) {
	if ok, err := s.agentExists(ctx, orgID, agentID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAgentNotFound
	}
	var rows []model.AgentMCPServer
	if err := s.db.WithContext(ctx).Where("org_id = ? AND agent_id = ?", orgID, agentID).
		Order("mcp_server_id ASC").Find(&rows).Error; err != nil {
		return nil, fmt.Errorf("list agent mcp server grants: %w", err)
	}
	result := make([]AgentGrant, 0, len(rows))
	for _, row := range rows {
		result = append(result, AgentGrant{MCPServerID: row.MCPServerID, State: row.State})
	}
	return result, nil
}

func (s *Service) AttachPersonal(ctx context.Context, orgID, userID, agentID, serverID uuid.UUID) error {
	if ok, err := s.agentExists(ctx, orgID, agentID); err != nil {
		return err
	} else if !ok {
		return ErrAgentNotFound
	}
	server, err := s.GetServer(ctx, orgID, serverID, &userID)
	if err != nil {
		return err
	}
	if server.Scope != model.MCPServerScopePersonal || server.OwnerUserID == nil || *server.OwnerUserID != userID {
		return ErrNotFound
	}
	row := model.UserAgentMCPServer{OrgID: orgID, UserID: userID, AgentID: agentID, MCPServerID: serverID}
	if err := s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "user_id"}, {Name: "agent_id"}, {Name: "mcp_server_id"}}, DoNothing: true,
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("attach personal mcp server: %w", err)
	}
	return nil
}

func (s *Service) DetachPersonal(ctx context.Context, orgID, userID, agentID, serverID uuid.UUID) error {
	if ok, err := s.agentExists(ctx, orgID, agentID); err != nil {
		return err
	} else if !ok {
		return ErrAgentNotFound
	}
	if _, err := s.GetServer(ctx, orgID, serverID, &userID); err != nil {
		return err
	}
	if err := s.db.WithContext(ctx).Where("org_id = ? AND user_id = ? AND agent_id = ? AND mcp_server_id = ?", orgID, userID, agentID, serverID).
		Delete(&model.UserAgentMCPServer{}).Error; err != nil {
		return fmt.Errorf("detach personal mcp server: %w", err)
	}
	return nil
}

func (s *Service) PersonalAgentServers(ctx context.Context, orgID, userID, agentID uuid.UUID) ([]model.MCPServer, error) {
	if ok, err := s.agentExists(ctx, orgID, agentID); err != nil {
		return nil, err
	} else if !ok {
		return nil, ErrAgentNotFound
	}
	servers := []model.MCPServer{}
	if err := s.db.WithContext(ctx).Table("mcp_servers ms").Select("ms.*").
		Joins("JOIN user_agent_mcp_servers uams ON uams.mcp_server_id = ms.id AND uams.org_id = ms.org_id").
		Where("ms.org_id = ? AND uams.user_id = ? AND uams.agent_id = ? AND ms.scope = ? AND ms.owner_user_id = ?", orgID, userID, agentID, model.MCPServerScopePersonal, userID).
		Order("ms.name ASC, ms.id ASC").Scan(&servers).Error; err != nil {
		return nil, fmt.Errorf("list personal agent mcp servers: %w", err)
	}
	return servers, nil
}

func (s *Service) teamExists(ctx context.Context, orgID, teamID uuid.UUID) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.Team{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, orgID).Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("load MCP team: %w", err)
	}
	return count == 1, nil
}

func (s *Service) agentExists(ctx context.Context, orgID, agentID uuid.UUID) (bool, error) {
	var count int64
	err := s.db.WithContext(ctx).Model(&model.Agent{}).
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").Count(&count).Error
	if err != nil {
		return false, fmt.Errorf("load MCP agent: %w", err)
	}
	return count == 1, nil
}

func (s *Service) agentTeam(ctx context.Context, orgID, agentID uuid.UUID) (uuid.UUID, error) {
	var agent model.Agent
	if err := s.db.WithContext(ctx).Select("id", "team_id").
		Where("id = ? AND org_id = ? AND status <> ?", agentID, orgID, "archived").First(&agent).Error; errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, ErrAgentNotFound
	} else if err != nil {
		return uuid.Nil, fmt.Errorf("load MCP agent team: %w", err)
	}
	return agent.TeamID, nil
}
