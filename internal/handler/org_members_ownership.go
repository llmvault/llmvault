package handler

import (
	"context"
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type transferOwnershipRequest struct {
	NewOwnerUserID string `json:"new_owner_user_id"`
}

// TransferOwnership handles POST /v1/orgs/current/transfer-ownership.
// @Summary Transfer organization ownership
// @Description Owner-only. Promotes the target member to owner and demotes the calling owner to admin, atomically. The org always retains at least one owner. The target must be an existing member and cannot be the caller.
// @Tags org-members
// @Accept json
// @Produce json
// @Param body body transferOwnershipRequest true "New owner user ID"
// @Success 200 {object} orgMemberResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/transfer-ownership [post]
func (h *OrgMemberHandler) TransferOwnership(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, callerID, callerRole, ok := h.callerContext(w, r)
	if !ok {
		return
	}
	// Defense in depth: the route also gates this with RequireOrgOwner.
	if !isOrgOwner(callerRole) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "owner role required"})
		return
	}

	var req transferOwnershipRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	newOwnerID, err := uuid.Parse(req.NewOwnerUserID)
	if err != nil || newOwnerID == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "new_owner_user_id must be a valid user id"})
		return
	}
	if newOwnerID == callerID {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "you already own this organization"})
		return
	}

	_, found, err := h.membership(ctx, org.ID, newOwnerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load member"})
		return
	}
	if !found {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "target user is not a member of this organization"})
		return
	}

	if err := h.transferOwnershipTx(ctx, org.ID, callerID, newOwnerID); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "transfer ownership", "org_id", org.ID, "new_owner", newOwnerID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to transfer ownership"})
		return
	}
	writeJSON(w, http.StatusOK, h.memberResponse(ctx, org.ID, newOwnerID, "owner"))
}

// transferOwnershipTx promotes the new owner and demotes the caller to admin in
// a single transaction, so the org is never left without an owner.
func (h *OrgMemberHandler) transferOwnershipTx(ctx context.Context, orgID, callerID, newOwnerID uuid.UUID) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.OrgMembership{}).
			Where("org_id = ? AND user_id = ?", orgID, newOwnerID).
			Update("role", "owner").Error; err != nil {
			return err
		}
		return tx.Model(&model.OrgMembership{}).
			Where("org_id = ? AND user_id = ?", orgID, callerID).
			Update("role", "admin").Error
	})
}

// DeleteOrg handles DELETE /v1/orgs/current.
// @Summary Delete the organization
// @Description Owner-only. Permanently deletes the organization and all of its data. Most child tables (agents, channels, sessions, API keys, credentials, …) are removed by database ON DELETE CASCADE; org_invites, org_memberships and usage rows lack cascade and are deleted explicitly first. This is irreversible.
// @Tags org-members
// @Produce json
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current [delete]
func (h *OrgMemberHandler) DeleteOrg(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, _, callerRole, ok := h.callerContext(w, r)
	if !ok {
		return
	}
	// Defense in depth: the route also gates this with RequireOrgOwner.
	if !isOrgOwner(callerRole) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "owner role required"})
		return
	}

	if err := h.deleteOrgTx(ctx, org.ID); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "delete org", "org_id", org.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete organization"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "deleted"})
}

// deleteOrgTx removes the org and everything under it. The three org FKs that
// lack ON DELETE CASCADE (org_invite_teams' parent org_invites, usage, and
// org_memberships) are deleted child-first inside the transaction; deleting the
// org row then cascades the remaining child tables.
func (h *OrgMemberHandler) deleteOrgTx(ctx context.Context, orgID uuid.UUID) error {
	return h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("org_id = ?", orgID).Delete(&model.OrgInviteTeam{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", orgID).Delete(&model.OrgInvite{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", orgID).Delete(&model.Usage{}).Error; err != nil {
			return err
		}
		if err := tx.Where("org_id = ?", orgID).Delete(&model.OrgMembership{}).Error; err != nil {
			return err
		}
		return tx.Where("id = ?", orgID).Delete(&model.Org{}).Error
	})
}
