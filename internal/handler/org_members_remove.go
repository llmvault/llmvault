package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// Remove handles DELETE /v1/orgs/current/members/{userID}.
// @Summary Remove a member from the organization
// @Description Removes a member from the org. Requires org admin. Only an owner may remove an owner, the last owner cannot be removed, and removal also strips the user's team and channel memberships within the org. Sessions the user created are retained (their created_by is nulled by the database).
// @Tags org-members
// @Produce json
// @Param userID path string true "Target user ID"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/members/{userID} [delete]
func (h *OrgMemberHandler) Remove(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, _, callerRole, ok := h.callerContext(w, r)
	if !ok {
		return
	}
	targetID, ok := orgMemberTargetID(w, r)
	if !ok {
		return
	}

	target, found, err := h.membership(ctx, org.ID, targetID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load member"})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "member not found"})
		return
	}
	if target.Role == "owner" && !isOrgOwner(callerRole) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "only an owner can remove an owner"})
		return
	}
	if target.Role == "owner" {
		owners, err := h.ownerCount(ctx, org.ID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to count owners"})
			return
		}
		if owners <= 1 {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "cannot remove the last owner"})
			return
		}
	}

	if err := h.removeMemberTx(ctx, org.ID, targetID); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "remove member", "org_id", org.ID, "user_id", targetID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to remove member"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "removed"})
}

// removeMemberTx deletes the user's org membership plus their team and channel
// memberships within the org, atomically. Channel members are scoped to the org
// via a subquery over the org's channels (channel_members has no org_id column).
func (h *OrgMemberHandler) removeMemberTx(ctx context.Context, orgID, userID uuid.UUID) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ? AND user_id = ?", orgID, userID).
			Delete(&model.TeamMember{}).Error; err != nil {
			return err
		}
		channelIDs := tx.Model(&model.Channel{}).Select("id").Where("org_id = ?", orgID)
		if err := tx.Where("user_id = ? AND channel_id IN (?)", userID, channelIDs).
			Delete(&model.ChannelMember{}).Error; err != nil {
			return err
		}
		return tx.Where("org_id = ? AND user_id = ?", orgID, userID).
			Delete(&model.OrgMembership{}).Error
	})
}
