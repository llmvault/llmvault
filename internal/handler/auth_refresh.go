package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// @Summary Refresh tokens
// @Description Exchanges a refresh token for new access and refresh tokens.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body refreshRequest true "Refresh parameters"
// @Success 200 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Router /auth/refresh [post]
func (h *AuthHandler) Refresh(w http.ResponseWriter, r *http.Request) {
	var req refreshRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token is required"})
		return
	}

	userID, _, err := auth.ValidateRefreshToken(h.signingKey, req.RefreshToken)
	if err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid refresh token"})
		return
	}

	tokenHash := hashToken(req.RefreshToken)
	now := time.Now()

	// Atomically claim this single-use token: only the caller whose UPDATE
	// flips revoked_at from NULL gets to rotate it. Concurrent presentations of
	// the same token (multiple tabs, the proxy, the stream-token route, or
	// another instance) lose the race and fall through to the grace window
	// instead of force-logging the user out.
	res := h.db.Model(&model.RefreshToken{}).
		Where("token_hash = ? AND revoked_at IS NULL", tokenHash).
		Updates(map[string]any{"revoked_at": &now})
	if res.Error != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "internal error"})
		return
	}

	if res.RowsAffected == 0 {
		// We did not win the rotation. Either the token was already rotated
		// (recently, by a concurrent refresh) or it is genuinely revoked.
		h.serveRefreshGrace(r.Context(), w, tokenHash)
		return
	}

	// We won the rotation; load the row we just revoked.
	var storedToken model.RefreshToken
	if err := h.db.Where("token_hash = ?", tokenHash).First(&storedToken).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token revoked or not found"})
		return
	}

	if now.After(storedToken.ExpiresAt) {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token expired"})
		return
	}

	var user model.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err != nil {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "user not found"})
		return
	}

	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	if len(memberships) == 0 {
		writeJSON(w, http.StatusForbidden, map[string]string{"error": "no organization memberships"})
		return
	}

	orgID := memberships[0].OrgID.String()
	role := memberships[0].Role
	if req.OrgID != "" {
		found := false
		for _, m := range memberships {
			if m.OrgID.String() == req.OrgID {
				orgID = req.OrgID
				role = m.Role
				found = true
				break
			}
		}
		if !found {
			writeJSON(w, http.StatusForbidden, map[string]string{"error": "not a member of the requested organization"})
			return
		}
	}

	resp, ok := h.mintAuthResponse(r.Context(), w, user, orgID, role)
	if !ok {
		return
	}

	// Record the replacement pair on the rotated row so a concurrent caller
	// presenting the same single-use token within the grace window receives the
	// same rotated pair (rather than a force-logout). Best-effort: a write
	// failure here only forfeits the grace window, not the rotation.
	if err := h.db.Model(&model.RefreshToken{}).
		Where("token_hash = ?", tokenHash).
		Updates(map[string]any{
			"replaced_at":               &now,
			"replaced_by_access_token":  resp.AccessToken,
			"replaced_by_refresh_token": resp.RefreshToken,
		}).Error; err != nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "failed to persist refresh grace window", "error", err)
	}

	writeJSON(w, http.StatusOK, resp)
}

// refreshGraceWindow is how long after a single-use refresh token has been
// rotated a concurrent presentation of the same token may still receive the
// rotated pair instead of being rejected.
const refreshGraceWindow = 30 * time.Second

// refreshGraceWaitTotal bounds how long a refresh that lost the rotation race
// waits for the winning request to finish persisting its replacement pair. This
// closes the visibility gap between the winner revoking the row and writing the
// replacement tokens, so a concurrent loser never sees a half-rotated row and
// force-logs the user out.
const (
	refreshGraceWaitTotal = 2 * time.Second
	refreshGraceWaitStep  = 20 * time.Millisecond
)

// serveRefreshGrace handles a refresh request that lost the rotation race. If
// the token was rotated within the grace window and the replacement pair is
// recorded, it returns that same pair so concurrent refreshes converge on one
// session instead of force-logging the user out. Otherwise it rejects.
func (h *AuthHandler) serveRefreshGrace(ctx context.Context, w http.ResponseWriter, tokenHash string) {
	deadline := time.Now().Add(refreshGraceWaitTotal)
	for {
		var storedToken model.RefreshToken
		if err := h.db.WithContext(ctx).Where("token_hash = ?", tokenHash).First(&storedToken).Error; err != nil {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token revoked or not found"})
			return
		}

		if storedToken.ReplacedAt != nil &&
			storedToken.ReplacedByAccessToken != "" &&
			storedToken.ReplacedByRefreshToken != "" {
			if time.Since(*storedToken.ReplacedAt) <= refreshGraceWindow {
				writeJSON(w, http.StatusOK, h.buildAuthResponse(
					ctx,
					h.refreshGraceUser(storedToken.UserID),
					storedToken.ReplacedByAccessToken,
					storedToken.ReplacedByRefreshToken,
				))
				return
			}
			// Replacement exists but the grace window has elapsed: reject.
			break
		}

		// No replacement recorded yet. If the row was revoked only just now, a
		// concurrent winner is likely still persisting its replacement — wait
		// briefly and re-check. Otherwise (revoked long ago, e.g. logout) reject
		// immediately.
		if storedToken.RevokedAt == nil || time.Since(*storedToken.RevokedAt) > refreshGraceWindow {
			break
		}
		if time.Now().After(deadline) {
			break
		}
		time.Sleep(refreshGraceWaitStep)
	}

	writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "refresh token revoked or not found"})
}

// refreshGraceUser loads the user backing a grace-window reuse. The org/plan
// detail in buildAuthResponse is re-derived from the DB, so we only need the ID
// and the fields userResponse surfaces.
func (h *AuthHandler) refreshGraceUser(userID uuid.UUID) model.User {
	var user model.User
	if err := h.db.Where("id = ?", userID).First(&user).Error; err == nil {
		return user
	}
	return model.User{ID: userID}
}

// Logout handles POST /auth/logout.
// @Summary Log out
// @Description Revokes a refresh token.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body logoutRequest true "Logout parameters"
// @Success 200 {object} statusResponse
// @Security BearerAuth
// @Router /auth/logout [post]
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	var req logoutRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}

	if req.RefreshToken == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "refresh_token is required"})
		return
	}

	tokenHash := hashToken(req.RefreshToken)
	now := time.Now()
	h.db.Model(&model.RefreshToken{}).Where("token_hash = ? AND revoked_at IS NULL", tokenHash).Update("revoked_at", &now)

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
