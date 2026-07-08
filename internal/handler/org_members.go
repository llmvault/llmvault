package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// OrgMemberHandler owns the org-membership lifecycle: changing a member's org
// role, removing a member, transferring ownership, and deleting the org. These
// operate on the org_memberships table and enforce the org's role invariants
// (notably: an org must always retain at least one owner).
type OrgMemberHandler struct {
	db *gorm.DB
}

// NewOrgMemberHandler constructs an OrgMemberHandler.
func NewOrgMemberHandler(db *gorm.DB) *OrgMemberHandler {
	return &OrgMemberHandler{db: db}
}

type patchMemberRoleRequest struct {
	Role string `json:"role"`
}

// orgRoleRank ranks org roles so escalation guards can compare tiers.
// owner > admin > member > viewer.
func orgRoleRank(role string) int {
	switch role {
	case "owner":
		return 3
	case "admin":
		return 2
	case "member":
		return 1
	case "viewer":
		return 0
	default:
		return -1
	}
}

func isValidOrgRole(role string) bool {
	return orgRoleRank(role) >= 0
}

// membership loads a user's membership in the org. found=false when the user is
// not a member (no row); err is set only on a real query failure.
func (h *OrgMemberHandler) membership(ctx context.Context, orgID, userID uuid.UUID) (model.OrgMembership, bool, error) {
	var m model.OrgMembership
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		First(&m).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.OrgMembership{}, false, nil
	}
	if err != nil {
		return model.OrgMembership{}, false, err
	}
	return m, true, nil
}

// ownerCount returns how many owners the org currently has. Used to protect the
// ≥1-owner invariant.
func (h *OrgMemberHandler) ownerCount(ctx context.Context, orgID uuid.UUID) (int64, error) {
	var n int64
	err := h.db.WithContext(ctx).
		Model(&model.OrgMembership{}).
		Where("org_id = ? AND role = ?", orgID, "owner").
		Count(&n).Error
	return n, err
}

// callerContext resolves the org, the acting user, and their org role. It writes
// the appropriate error response and returns ok=false when any is missing.
func (h *OrgMemberHandler) callerContext(w http.ResponseWriter, r *http.Request) (*model.Org, uuid.UUID, string, bool) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "missing organization context"})
		return nil, uuid.Nil, "", false
	}
	callerID, ok := currentRequestUserID(r.Context())
	if !ok || callerID == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "user context is required"})
		return nil, uuid.Nil, "", false
	}
	caller, found, err := h.membership(r.Context(), org.ID, *callerID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load membership"})
		return nil, uuid.Nil, "", false
	}
	if !found {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "not a member of this organization"})
		return nil, uuid.Nil, "", false
	}
	return org, *callerID, caller.Role, true
}

func orgMemberTargetID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(chi.URLParam(r, "userID"))
	if err != nil || id == uuid.Nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid user id"})
		return uuid.Nil, false
	}
	return id, true
}

// PatchRole handles PATCH /v1/orgs/current/members/{userID}/role.
// @Summary Change a member's organization role
// @Description Changes a member's org role. Requires org admin. Only an owner may grant or revoke the owner role, no caller may grant a role above their own tier, the last owner cannot be demoted, and a caller cannot change their own role.
// @Tags org-members
// @Accept json
// @Produce json
// @Param userID path string true "Target user ID"
// @Param body body patchMemberRoleRequest true "New role: owner, admin, member, or viewer"
// @Success 200 {object} orgMemberResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/members/{userID}/role [patch]
func (h *OrgMemberHandler) PatchRole(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, callerID, callerRole, ok := h.callerContext(w, r)
	if !ok {
		return
	}
	targetID, ok := orgMemberTargetID(w, r)
	if !ok {
		return
	}
	var req patchMemberRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if !isValidOrgRole(req.Role) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "role must be one of: owner, admin, member, viewer"})
		return
	}
	if targetID == callerID {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "you cannot change your own role"})
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

	if code, msg, ok := h.authorizeRoleChange(ctx, org.ID, callerRole, target.Role, req.Role); !ok {
		writeJSON(w, code, errorResponse{Error: msg})
		return
	}

	if req.Role != target.Role {
		if err := h.db.WithContext(ctx).
			Model(&model.OrgMembership{}).
			Where("org_id = ? AND user_id = ?", org.ID, targetID).
			Update("role", req.Role).Error; err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "update member role", "org_id", org.ID, "user_id", targetID, "error", err)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update member role"})
			return
		}
		target.Role = req.Role
	}
	writeJSON(w, http.StatusOK, h.memberResponse(ctx, org.ID, targetID, target.Role))
}

// authorizeRoleChange applies the role-change invariants and returns the HTTP
// status + message to emit when the change is disallowed.
func (h *OrgMemberHandler) authorizeRoleChange(ctx context.Context, orgID uuid.UUID, callerRole, currentRole, newRole string) (int, string, bool) {
	// Only an owner may grant the owner role.
	if newRole == "owner" && !isOrgOwner(callerRole) {
		return http.StatusForbidden, "only an owner can grant the owner role", false
	}
	// Revoking an existing owner is owner-only and must preserve one owner.
	if currentRole == "owner" && newRole != "owner" {
		if !isOrgOwner(callerRole) {
			return http.StatusForbidden, "only an owner can change an owner's role", false
		}
		owners, err := h.ownerCount(ctx, orgID)
		if err != nil {
			return http.StatusInternalServerError, "failed to count owners", false
		}
		if owners <= 1 {
			return http.StatusBadRequest, "cannot demote the last owner", false
		}
	}
	// No caller may grant a role above their own tier.
	if orgRoleRank(newRole) > orgRoleRank(callerRole) {
		return http.StatusForbidden, "cannot assign a role higher than your own", false
	}
	return 0, "", true
}

// memberResponse builds the response DTO for a membership, preloading the user.
func (h *OrgMemberHandler) memberResponse(ctx context.Context, orgID, userID uuid.UUID, role string) orgMemberResponse {
	var m model.OrgMembership
	_ = h.db.WithContext(ctx).Preload("User").
		Where("org_id = ? AND user_id = ?", orgID, userID).
		First(&m).Error
	return orgMemberResponse{
		UserID:   userID.String(),
		Email:    m.User.Email,
		Name:     m.User.Name,
		Role:     role,
		JoinedAt: m.CreatedAt.Format(time.RFC3339),
	}
}
