// Package teamprovision manages the connection, skill, and knowledge grants
// inherited by every agent assigned to a team.
package teamprovision

import (
	"context"
	"errors"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

var (
	ErrTeamNotFound       = errors.New("teamprovision: team not found in org")
	ErrConnectionNotFound = errors.New("teamprovision: connection not found in org")
	ErrConnectionRequired = errors.New("teamprovision: connection is required by active team agents")
	ErrSkillNotFound      = errors.New("teamprovision: org skill not found")
	ErrSourceNotFound     = errors.New("teamprovision: rag source not found in org")
)

func SourceGrantedToTeam(ctx context.Context, db *gorm.DB, teamID, ragSourceID uuid.UUID) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&model.TeamRagSource{}).
		Where("team_id = ? AND rag_source_id = ?", teamID, ragSourceID).
		Count(&count).Error
	return count > 0, err
}

func GrantedSourceIDs(ctx context.Context, db *gorm.DB, teamID uuid.UUID) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	err := db.WithContext(ctx).Model(&model.TeamRagSource{}).
		Where("team_id = ?", teamID).
		Pluck("rag_source_id", &ids).Error
	return ids, err
}

func teamInOrg(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&model.Team{}).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, orgID).
		Count(&count).Error
	return count > 0, err
}
