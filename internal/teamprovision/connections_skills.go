package teamprovision

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

func GrantConnection(ctx context.Context, db *gorm.DB, orgID, teamID, connectionID uuid.UUID, grantedBy *uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	row := model.TeamConnectionGrant{ID: uuid.New(), OrgID: orgID, TeamID: teamID, GrantedBy: grantedBy}
	var connection model.Connection
	err = db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, orgID).
		First(&connection).Error
	if err == nil {
		row.ConnectionID = &connectionID
		return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	var databaseConnection model.DatabaseConnection
	err = db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND revoked_at IS NULL", connectionID, orgID).
		First(&databaseConnection).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return ErrConnectionNotFound
	}
	if err != nil {
		return err
	}
	row.DatabaseConnectionID = &connectionID
	return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func RevokeConnection(ctx context.Context, db *gorm.DB, orgID, teamID, connectionID uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	required, err := connectionRequiredByTeamAgents(ctx, db, orgID, teamID, connectionID)
	if err != nil {
		return err
	}
	if required {
		return ErrConnectionRequired
	}
	return db.WithContext(ctx).
		Where("org_id = ? AND team_id = ? AND (connection_id = ? OR database_connection_id = ?)", orgID, teamID, connectionID, connectionID).
		Delete(&model.TeamConnectionGrant{}).Error
}

func ConnectionGrants(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID) ([]model.TeamConnectionGrant, error) {
	rows := []model.TeamConnectionGrant{}
	err := db.WithContext(ctx).
		Preload("Connection.Integration").Preload("DatabaseConnection").
		Where("org_id = ? AND team_id = ?", orgID, teamID).
		Order("created_at ASC, id ASC").Find(&rows).Error
	return rows, err
}

func GrantSkill(ctx context.Context, db *gorm.DB, orgID, teamID, skillID uuid.UUID, grantedBy *uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	var count int64
	err = db.WithContext(ctx).Model(&model.Skill{}).
		Where("id = ? AND org_id = ? AND team_id IS NULL AND status = ?", skillID, orgID, model.SkillStatusPublished).
		Count(&count).Error
	if err != nil {
		return err
	}
	if count == 0 {
		return ErrSkillNotFound
	}
	row := model.TeamSkillGrant{ID: uuid.New(), OrgID: orgID, TeamID: teamID, SkillID: skillID, GrantedBy: grantedBy}
	return db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&row).Error
}

func RevokeSkill(ctx context.Context, db *gorm.DB, orgID, teamID, skillID uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	return db.WithContext(ctx).
		Where("org_id = ? AND team_id = ? AND skill_id = ?", orgID, teamID, skillID).
		Delete(&model.TeamSkillGrant{}).Error
}

func SkillGrants(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID) ([]model.TeamSkillGrant, error) {
	rows := []model.TeamSkillGrant{}
	err := db.WithContext(ctx).Preload("Skill").
		Where("org_id = ? AND team_id = ?", orgID, teamID).
		Order("created_at ASC, id ASC").Find(&rows).Error
	return rows, err
}

func connectionRequiredByTeamAgents(ctx context.Context, db *gorm.DB, orgID, teamID, connectionID uuid.UUID) (bool, error) {
	provider := ""
	var connection model.Connection
	err := db.WithContext(ctx).Preload("Integration").Where("id = ? AND org_id = ?", connectionID, orgID).First(&connection).Error
	if err == nil {
		provider = connection.Integration.Provider
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return false, err
	}
	if provider == "" {
		var databaseConnection model.DatabaseConnection
		if err := db.WithContext(ctx).Where("id = ? AND org_id = ?", connectionID, orgID).First(&databaseConnection).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return false, nil
			}
			return false, err
		}
		provider = databaseConnection.Provider
	}
	var count int64
	err = db.WithContext(ctx).Model(&model.Agent{}).
		Joins("JOIN agent_catalog ON agent_catalog.id = agents.agent_catalog_id").
		Where("agents.org_id = ? AND agents.team_id = ? AND agents.status <> ?", orgID, teamID, "archived").
		Where("? = ANY(agent_catalog.required_connections)", provider).
		Count(&count).Error
	return count > 0, err
}
