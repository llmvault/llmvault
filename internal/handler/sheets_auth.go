package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Sheets are channel-scoped: a sheet's visibility follows its channel's access.
// The REST surface authorizes every sheet against its channel using the same
// canUseChannel predicate as the rest of the API, so the HTTP path and the agent
// MCP path cannot drift. Access is strictly team-scoped: org managers and
// API-key callers pass; a human member must be able to use the sheet's channel
// (team membership). A denied request 404s so a sheet in a channel the caller
// cannot see is indistinguishable from a missing one.

// canUseSheetChannel reports whether the current request may act on sheets in
// channelID. Mirrors SessionHandler.canUseChannel: it delegates to the shared
// canUseChannel predicate so the HTTP and agent paths cannot drift.
func (h *SheetsHandler) canUseSheetChannel(ctx context.Context, orgID, channelID uuid.UUID) bool {
	var channel model.Channel
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ?", channelID, orgID).
		First(&channel).Error; err != nil {
		return false
	}
	userID := sheetsRequestUserID(ctx)
	orgRole, err := h.sheetsOrgRole(ctx, orgID, userID)
	if err != nil {
		return false
	}
	return canUseChannel(ctx, h.db, channel, orgRole, userID, isAPIKeyRequest(ctx))
}

func sheetsRequestUserID(ctx context.Context) *uuid.UUID {
	if user, ok := middleware.UserFromContext(ctx); ok && user != nil {
		return &user.ID
	}
	return nil
}

func (h *SheetsHandler) sheetsOrgRole(ctx context.Context, orgID uuid.UUID, userID *uuid.UUID) (string, error) {
	if userID == nil {
		return "", nil
	}
	var membership model.OrgMembership
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, *userID).
		First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return membership.Role, err
}

// RequireChannelAccess is the middleware guarding every /sheets/{sheetID}/...
// route: it loads the addressed sheet and 404s unless the requester can use its
// channel. Mounted on the {sheetID} group so no nested route can bypass it.
func (h *SheetsHandler) RequireChannelAccess(next http.Handler) http.Handler {
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
		if !h.canUseSheetChannel(r.Context(), org.ID, sheet.ChannelID) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
			return
		}
		next.ServeHTTP(w, r)
	})
}
