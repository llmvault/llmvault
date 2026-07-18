package handler

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func currentRequestUserID(ctx context.Context) (*uuid.UUID, bool) {
	raw := strings.TrimSpace(middleware.UserID(ctx))
	if raw == "" {
		return nil, false
	}
	id, err := uuid.Parse(raw)
	if err != nil || id == uuid.Nil {
		return nil, false
	}
	return &id, true
}

func isAPIKeyRequest(ctx context.Context) bool {
	_, ok := middleware.APIKeyClaimsFromContext(ctx)
	return ok
}
func isOrgManager(role string) bool { return role == "owner" || role == "admin" }
func isOrgOwner(role string) bool   { return role == "owner" }
func defaultString(value, fallback string) string {
	if strings.TrimSpace(value) == "" {
		return fallback
	}
	return value
}
func formatUUIDPtr(id *uuid.UUID) *string {
	if id == nil || *id == uuid.Nil {
		return nil
	}
	value := id.String()
	return &value
}
func formatTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	text := value.UTC().Format(time.RFC3339)
	return &text
}

// canUseTeam is the HTTP entry point for team-resource access. API-key callers
// retain org-wide access; a human must be an active org member and either an
// org manager or active member of the addressed team.
func canUseTeam(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID, userID *uuid.UUID, apiKey bool) bool {
	if apiKey {
		return true
	}
	role, err := orgRoleForUser(ctx, db, orgID, userID)
	if err != nil || userID == nil || role == "" {
		return false
	}
	actor := &access.Actor{UserID: *userID, OrgID: orgID, OrgRole: role}
	ok, err := actor.CanManageTeamResource(ctx, db, teamID)
	return err == nil && ok
}

// canManageTeamResource is retained as the common handler predicate for
// mutating team-owned resources.
func canManageTeamResource(ctx context.Context, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID, role string, teamID uuid.UUID) bool {
	if userID == nil {
		return false
	}
	actor := &access.Actor{UserID: *userID, OrgID: orgID, OrgRole: role}
	ok, err := actor.CanManageTeamResource(ctx, db, teamID)
	return err == nil && ok
}

func visibleTeamSubquery(db *gorm.DB, userID *uuid.UUID) *gorm.DB {
	q := db.Model(&model.TeamMember{}).
		Select("team_members.team_id").
		Joins("JOIN teams ON teams.id = team_members.team_id AND teams.archived_at IS NULL").
		Where("team_members.deactivated_at IS NULL")
	if userID == nil {
		return q.Where("1 = 0")
	}
	return q.Where("team_members.user_id = ?", *userID)
}

// orgRoleForUser resolves a user's active org role, returning an empty role
// when the user is absent or no longer a member.
func orgRoleForUser(ctx context.Context, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) (string, error) {
	if userID == nil {
		return "", nil
	}
	var membership model.OrgMembership
	err := db.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND deactivated_at IS NULL", orgID, *userID).
		First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return membership.Role, err
}

func actorSeesOrgWide(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (bool, *uuid.UUID, error) {
	if isAPIKeyRequest(ctx) {
		return true, nil, nil
	}
	userID, _ := currentRequestUserID(ctx)
	role, err := orgRoleForUser(ctx, db, orgID, userID)
	if err != nil {
		return false, userID, err
	}
	return isOrgManager(role), userID, nil
}

// visibleSessionIDSubquery exposes team sessions, plus sessions explicitly
// created by or shared with the caller. Managers are handled by the caller.
func visibleSessionIDSubquery(db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) *gorm.DB {
	q := db.Model(&model.Session{}).Select("sessions.id").Where("sessions.org_id = ?", orgID)
	if userID == nil {
		return q.Where("1 = 0")
	}
	participants := db.Model(&model.SessionParticipant{}).Select("session_id").Where("user_id = ?", *userID)
	return q.Where("sessions.team_id IN (?) OR sessions.created_by = ? OR sessions.id IN (?)", visibleTeamSubquery(db, userID), *userID, participants)
}

func visibleTeamRAGSourceSubquery(db *gorm.DB, userID *uuid.UUID) *gorm.DB {
	return db.Model(&model.TeamRagSource{}).
		Select("team_rag_sources.rag_source_id").
		Where("team_rag_sources.team_id IN (?)", visibleTeamSubquery(db, userID)).
		Where("team_rag_sources.removed_at IS NULL")
}

func usableRagSourceIDs(ctx context.Context, db *gorm.DB, orgID uuid.UUID, userID *uuid.UUID) ([]string, error) {
	var ids []uuid.UUID
	err := db.WithContext(ctx).Model(&model.TeamRagSource{}).Distinct("team_rag_sources.rag_source_id").Where("team_rag_sources.org_id = ?", orgID).Where("team_rag_sources.team_id IN (?)", visibleTeamSubquery(db, userID)).Pluck("team_rag_sources.rag_source_id", &ids).Error
	if err != nil {
		return nil, err
	}
	out := make([]string, len(ids))
	for i := range ids {
		out[i] = ids[i].String()
	}
	return out, nil
}
