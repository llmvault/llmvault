package handler

import (
	"context"
	"net/http"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Sheets are team-scoped. A denied request is a 404, so a sheet in another
// team is indistinguishable from a missing one.

func (h *SheetsHandler) canUseSheetTeam(ctx context.Context, orgID, teamID uuid.UUID) bool {
	if isAPIKeyRequest(ctx) {
		return true
	}
	userID := sheetsRequestUserID(ctx)
	if userID == nil {
		return false
	}
	var membership model.OrgMembership
	if err := h.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ? AND deactivated_at IS NULL", orgID, *userID).
		First(&membership).Error; err != nil {
		return false
	}
	actor := &access.Actor{UserID: *userID, OrgID: orgID, OrgRole: membership.Role}
	ok, err := actor.CanManageTeamResource(ctx, h.db, teamID)
	return err == nil && ok
}

func sheetsRequestUserID(ctx context.Context) *uuid.UUID {
	if user, ok := middleware.UserFromContext(ctx); ok && user != nil {
		return &user.ID
	}
	return nil
}

// RequireTeamAccess is the middleware guarding every /sheets/{sheetID}/...
// route. Mounted on the {sheetID} group so no nested route can bypass it.
func (h *SheetsHandler) RequireTeamAccess(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		org, ok := h.requireSheetsOrg(w, r)
		if !ok {
			return
		}
		sheetID, ok := sheetsPathUUID(w, r, "sheetID")
		if !ok {
			return
		}
		sheet, err := h.svc.SheetByID(r.Context(), org.ID, sheetID)
		if err != nil {
			writeSheetsError(w, r, err)
			return
		}
		if !h.canUseSheetTeam(r.Context(), org.ID, sheet.TeamID) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
