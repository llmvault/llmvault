package handler

import (
	"context"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/apps"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Get handles GET /v1/apps/{appID}.
// @Summary Get an app with its version history
// @Tags apps
// @Produce json
// @Param appID path string true "App ID"
// @Success 200 {object} appDetailResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/apps/{appID} [get]
func (h *AppsHandler) Get(w http.ResponseWriter, r *http.Request) {
	org, app, ok := h.requireApp(w, r)
	if !ok {
		return
	}
	versions, err := h.svc.ListVersions(r.Context(), org.ID, app.ID, 20)
	if err != nil {
		writeAppsError(w, r, err)
		return
	}
	versionViews := make([]appVersionView, 0, len(versions))
	for _, version := range versions {
		versionViews = append(versionViews, appVersionViewFrom(version))
	}
	writeJSON(w, http.StatusOK, map[string]any{"app": appViewFrom(app), "versions": versionViews})
}

// Archive handles DELETE /v1/apps/{appID}: soft-archive plus a best-effort
// sandbox stop.
// @Summary Archive an app
// @Tags apps
// @Param appID path string true "App ID"
// @Success 204
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/apps/{appID} [delete]
func (h *AppsHandler) Archive(w http.ResponseWriter, r *http.Request) {
	org, app, ok := h.requireApp(w, r)
	if !ok {
		return
	}
	if err := h.svc.ArchiveApp(r.Context(), org.ID, app.ID); err != nil {
		writeAppsError(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// requireApp resolves org + app and authorizes the app's channel; a denied or
// missing app 404s identically.
func (h *AppsHandler) requireApp(w http.ResponseWriter, r *http.Request) (*model.Org, *model.App, bool) {
	org, ok := h.requireAppsOrg(w, r)
	if !ok {
		return nil, nil, false
	}
	appID, err := uuid.Parse(chi.URLParam(r, "appID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "appID must be a uuid"})
		return nil, nil, false
	}
	app, err := h.svc.GetApp(r.Context(), org.ID, appID)
	if err != nil {
		writeAppsError(w, r, err)
		return nil, nil, false
	}
	if !h.canUseAppChannel(r.Context(), org.ID, app.ChannelID) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
		return nil, nil, false
	}
	return org, app, true
}

func (h *AppsHandler) requireAppsOrg(w http.ResponseWriter, r *http.Request) (*model.Org, bool) {
	if h == nil || h.svc == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "apps are not configured"})
		return nil, false
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return nil, false
	}
	return org, true
}

// canUseAppChannel mirrors canUseSheetChannel: org managers pass, API-key
// callers only reach channels open to any org member, everyone else must be
// able to use the channel (team membership).
func (h *AppsHandler) canUseAppChannel(ctx context.Context, orgID, channelID uuid.UUID) bool {
	var channel model.Channel
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", channelID, orgID).
		First(&channel).Error; err != nil {
		return false
	}
	if isAPIKeyRequest(ctx) {
		return channel.Origin == "external" || channel.TeamID == nil
	}
	var userID *uuid.UUID
	if user, ok := middleware.UserFromContext(ctx); ok && user != nil {
		userID = &user.ID
	}
	if userID == nil {
		return false
	}
	role, err := h.appsOrgRole(ctx, orgID, *userID)
	if err != nil || role == "" {
		return false
	}
	return canUseChannel(ctx, h.db, channel, role, userID, false)
}

func (h *AppsHandler) appsOrgRole(ctx context.Context, orgID, userID uuid.UUID) (string, error) {
	var membership model.OrgMembership
	err := h.db.WithContext(ctx).
		Where("org_id = ? AND user_id = ?", orgID, userID).
		First(&membership).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return "", nil
	}
	return membership.Role, err
}

// writeAppsError maps service errors onto HTTP statuses: unknown scope →
// 404, taken slug → 409, caller mistakes → 400.
func writeAppsError(w http.ResponseWriter, r *http.Request, err error) {
	var validationErr *apps.ValidationError
	switch {
	case errors.Is(err, apps.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "not found"})
	case errors.Is(err, apps.ErrSlugTaken):
		writeJSON(w, http.StatusConflict, errorResponse{Error: err.Error()})
	case errors.Is(err, apps.ErrNotDeployed):
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: err.Error()})
	case errors.As(err, &validationErr):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
	default:
		logging.Capture(r.Context(), err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "apps request failed"})
	}
}
