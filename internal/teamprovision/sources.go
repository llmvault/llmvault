package teamprovision

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

// GrantSource adds ragSourceID to teamID's knowledge allowlist. Idempotent. It
// validates the team belongs to orgID and the source belongs to orgID.
// grantedBy records the acting user (nil for API-key callers).
//
// Errors: ErrTeamNotFound, ErrSourceNotFound.
func GrantSource(ctx context.Context, db *gorm.DB, orgID, teamID, ragSourceID uuid.UUID, grantedBy *uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	ok, err = sourceInOrg(ctx, db, orgID, ragSourceID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrSourceNotFound
	}
	row := model.TeamRagSource{
		OrgID:       orgID,
		TeamID:      teamID,
		RagSourceID: ragSourceID,
		GrantedBy:   grantedBy,
	}
	return db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "team_id"}, {Name: "rag_source_id"}},
		DoNothing: true,
	}).Create(&row).Error
}

// RevokeSource removes ragSourceID from teamID's knowledge allowlist.
// Idempotent. Validates the team belongs to orgID.
func RevokeSource(ctx context.Context, db *gorm.DB, orgID, teamID, ragSourceID uuid.UUID) error {
	ok, err := teamInOrg(ctx, db, orgID, teamID)
	if err != nil {
		return err
	}
	if !ok {
		return ErrTeamNotFound
	}
	return db.WithContext(ctx).
		Where("org_id = ? AND team_id = ? AND rag_source_id = ?", orgID, teamID, ragSourceID).
		Delete(&model.TeamRagSource{}).Error
}

// UsableSourceIDsForTeams returns the distinct rag source ids granted to any of
// the given teams within orgID. This is the team-derived replacement for the
// old channel-scoped "usable sources" resolution used by RAG retrieval: resolve
// the teams a caller may reach, then call this. Returns an empty slice (never
// nil) so callers can treat "no teams" as deny-by-default.
func UsableSourceIDsForTeams(ctx context.Context, db *gorm.DB, orgID uuid.UUID, teamIDs []uuid.UUID) ([]uuid.UUID, error) {
	ids := []uuid.UUID{}
	if len(teamIDs) == 0 {
		return ids, nil
	}
	err := db.WithContext(ctx).Model(&model.TeamRagSource{}).
		Distinct("rag_source_id").
		Where("org_id = ? AND team_id IN ?", orgID, teamIDs).
		Pluck("rag_source_id", &ids).Error
	return ids, err
}

// sourceInOrg reports whether ragSourceID names a source owned by orgID.
func sourceInOrg(ctx context.Context, db *gorm.DB, orgID, ragSourceID uuid.UUID) (bool, error) {
	var count int64
	err := db.WithContext(ctx).Model(&ragmodel.RAGSource{}).
		Where("id = ? AND org_id = ?", ragSourceID, orgID).
		Count(&count).Error
	return count > 0, err
}
