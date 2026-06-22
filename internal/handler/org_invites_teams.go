package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func (h *OrgInviteHandler) normalizeInviteTeamIDs(ctx context.Context, w http.ResponseWriter, orgID uuid.UUID, raw []string) ([]uuid.UUID, bool) {
	if len(raw) == 0 {
		return nil, true
	}
	ids := make([]uuid.UUID, 0, len(raw))
	seen := map[uuid.UUID]bool{}
	for _, value := range raw {
		id, err := uuid.Parse(value)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "team_ids must contain only uuids"})
			return nil, false
		}
		if seen[id] {
			continue
		}
		seen[id] = true
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return ids, true
	}
	var count int64
	if err := h.db.WithContext(ctx).
		Model(&model.Team{}).
		Where("org_id = ? AND id IN ? AND archived_at IS NULL", orgID, ids).
		Count(&count).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate team_ids"})
		return nil, false
	}
	if count != int64(len(ids)) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "team_ids must belong to active teams in this org"})
		return nil, false
	}
	return ids, true
}

func inviteTeamRows(orgID, inviteID uuid.UUID, teamIDs []uuid.UUID) []model.OrgInviteTeam {
	rows := make([]model.OrgInviteTeam, len(teamIDs))
	for i, teamID := range teamIDs {
		rows[i] = model.OrgInviteTeam{OrgID: orgID, OrgInviteID: inviteID, TeamID: teamID}
	}
	return rows
}
