package handler

import (
	"context"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// Remove handles DELETE /v1/orgs/current/members/{userID}.
// @Summary Deactivate a member of the organization
// @Description Deactivates (archives) a member of the org. Requires org admin. Only an owner may remove an owner, the last owner cannot be removed. Deactivation is a soft operation: the org/team/channel membership rows are retained with deactivated_at set (preserving the audit trail), the member's API keys are revoked, and the member is treated as no longer a member by all membership checks. The users row is never deleted. Sessions the user created are retained.
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

// removeMemberTx soft-deactivates the user's org membership plus their team and
// channel memberships within the org, atomically, and revokes the access those
// memberships conferred. Nothing is hard-deleted: every row is retained with
// deactivated_at set so the audit trail is preserved, and membership predicates
// treat deactivated_at IS NOT NULL as "no longer a member". Channel members are
// scoped to the org via a subquery over the org's channels (channel_members has
// no org_id column).
//
// Access revoked on deactivation:
//   - API keys the member created for this org are revoked (revoked_at set), so
//     they can no longer bypass team/channel checks org-wide via their key.
//   - The member's Nango connections in this org are revoked, so their OAuth
//     credentials can no longer be used by agents/webhooks after removal.
func (h *OrgMemberHandler) removeMemberTx(ctx context.Context, orgID, userID uuid.UUID) error {
	now := time.Now()
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.TeamMember{}).
			Where("org_id = ? AND user_id = ? AND deactivated_at IS NULL", orgID, userID).
			Update("deactivated_at", now).Error; err != nil {
			return err
		}
		// Revoke API keys the member created for this org so a removed employee
		// cannot retain org-wide access through a still-valid key.
		if err := tx.Model(&model.APIKey{}).
			Where("org_id = ? AND created_by = ? AND revoked_at IS NULL", orgID, userID).
			Update("revoked_at", now).Error; err != nil {
			return err
		}
		// Revoke the member's Nango connections in this org so their OAuth
		// credentials can no longer be used by agents or inbound webhooks.
		if err := tx.Model(&model.Connection{}).
			Where("org_id = ? AND user_id = ? AND revoked_at IS NULL", orgID, userID).
			Update("revoked_at", now).Error; err != nil {
			return err
		}
		return tx.Model(&model.OrgMembership{}).
			Where("org_id = ? AND user_id = ? AND deactivated_at IS NULL", orgID, userID).
			Update("deactivated_at", now).Error
	})
}
