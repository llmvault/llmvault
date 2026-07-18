package handler

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *TeamHandler) loadTeamForRequest(w http.ResponseWriter, r *http.Request) (model.Team, bool) {
	org, ok := orgForTeamRequest(w, r)
	if !ok {
		return model.Team{}, false
	}
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return model.Team{}, false
	}
	var team model.Team
	err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, org.ID).
		First(&team).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return model.Team{}, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load team"})
		return model.Team{}, false
	}
	// Managers and API keys always pass; a plain member may only reach a team
	// they belong to. Mutating routes (update/archive/member management) are
	// additionally route-gated to admins, so this only relaxes the member GET.
	if !callerCanAccessTeam(r.Context(), h.db, org.ID, team.ID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return model.Team{}, false
	}
	return team, true
}

// callerCanAccessTeam reports whether the current request may read the given
// team: API keys and org managers see every team; a plain member sees only the
// teams they are an active member of.
func callerCanAccessTeam(ctx context.Context, db *gorm.DB, orgID, teamID uuid.UUID) bool {
	if isAPIKeyRequest(ctx) {
		return true
	}
	userID, _ := currentRequestUserID(ctx)
	role, _ := orgRoleForUser(ctx, db, orgID, userID)
	return canManageTeamResource(ctx, db, orgID, userID, role, teamID)
}

func orgForTeamRequest(w http.ResponseWriter, r *http.Request) (*model.Org, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, false
	}
	return org, true
}

func teamIDFromRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	teamID, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid team id"})
		return uuid.Nil, false
	}
	return teamID, true
}

// defaultTeamName is the fallback for system provisioning and additional org
// creation. Initial signup leaves the org teamless until onboarding collects a
// user-provided name.
const defaultTeamName = "General"

func normalizeTeamName(raw string) string {
	return strings.Join(strings.Fields(strings.TrimSpace(raw)), " ")
}

func validTeamMemberRole(role string) bool {
	return role == "owner" || role == "member"
}

func (h *TeamHandler) teamResponses(ctx context.Context, teams []model.Team) []teamResponse {
	counts := h.teamCounts(ctx, teamIDs(teams))
	out := make([]teamResponse, len(teams))
	for i, team := range teams {
		out[i] = teamToResponse(team, counts.members[team.ID])
	}
	return out
}

func (h *TeamHandler) teamResponse(ctx context.Context, team model.Team) teamResponse {
	counts := h.teamCounts(ctx, []uuid.UUID{team.ID})
	return teamToResponse(team, counts.members[team.ID])
}

type teamCountMaps struct {
	members map[uuid.UUID]int64
}

func (h *TeamHandler) teamCounts(ctx context.Context, ids []uuid.UUID) teamCountMaps {
	out := teamCountMaps{
		members: make(map[uuid.UUID]int64, len(ids)),
	}
	if len(ids) == 0 {
		return out
	}
	type countRow struct {
		TeamID uuid.UUID
		Count  int64
	}
	var memberRows []countRow
	_ = h.db.WithContext(ctx).
		Model(&model.TeamMember{}).
		Select("team_id, count(*) AS count").
		Where("team_id IN ? AND deactivated_at IS NULL", ids).
		Group("team_id").
		Scan(&memberRows).Error
	for _, row := range memberRows {
		out.members[row.TeamID] = row.Count
	}
	return out
}

func teamIDs(teams []model.Team) []uuid.UUID {
	ids := make([]uuid.UUID, len(teams))
	for i, team := range teams {
		ids[i] = team.ID
	}
	return ids
}

func teamToResponse(team model.Team, memberCount int64) teamResponse {
	return teamResponse{
		ID:          team.ID.String(),
		OrgID:       team.OrgID.String(),
		Name:        team.Name,
		Description: team.Description,
		CreatedBy:   formatUUIDPtr(team.CreatedBy),
		MemberCount: memberCount,
		ArchivedAt:  formatTimePtr(team.ArchivedAt),
		CreatedAt:   team.CreatedAt.Format(time.RFC3339),
		UpdatedAt:   team.UpdatedAt.Format(time.RFC3339),
	}
}
