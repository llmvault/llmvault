package handler

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// Me handles GET /auth/me.
// @Summary Get current user
// @Description Returns the current user and their organization memberships.
// @Tags auth
// @Produce json
// @Success 200 {object} meResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /auth/me [get]
func (h *AuthHandler) Me(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var user model.User
	if err := h.db.Where("id = ?", claims.UserID).First(&user).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	var memberships []model.OrgMembership
	h.db.Preload("Org").Where("user_id = ?", user.ID).Find(&memberships)

	plans := loadPlans(r.Context(), h.db, memberships)

	orgs := make([]orgMemberDTO, 0, len(memberships))
	for _, m := range memberships {
		dto := orgMemberDTO{
			ID:      m.OrgID.String(),
			Name:    m.Org.Name,
			Role:    m.Role,
			BYOK:    m.Org.BYOK,
			LogoURL: m.Org.LogoURL,
			Plan:    plans[m.Org.PlanSlug],
		}
		if m.Role == "owner" || m.Role == "admin" {
			balance, err := h.credits.Balance(m.OrgID)
			if err != nil {
				writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to load balance"})
				return
			}
			dto.Credits = &balance
		}
		orgs = append(orgs, dto)
	}

	isPlatformAdmin := len(h.platformAdminEmails) > 0 && h.platformAdminEmails[user.Email]

	writeJSON(w, http.StatusOK, meResponse{
		User: userResponse{
			ID:             user.ID.String(),
			Email:          user.Email,
			Name:           user.Name,
			AvatarURL:      user.AvatarURL,
			EmailConfirmed: user.EmailConfirmedAt != nil,
		},
		Orgs:            orgs,
		IsPlatformAdmin: isPlatformAdmin,
	})
}

type confirmEmailRequest struct {
	Email string `json:"email"`
	Code  string `json:"code"`
}

type resendConfirmationRequest struct {
	Email string `json:"email"`
}

type forgotPasswordRequest struct {
	Email string `json:"email"`
}

type resetPasswordRequest struct {
	Token       string `json:"token"`
	NewPassword string `json:"new_password"`
}

type changePasswordRequest struct {
	CurrentPassword string `json:"current_password"`
	NewPassword     string `json:"new_password"`
}

// UpdateProfile handles PATCH /auth/me.
// @Summary Update current user profile
// @Description Updates the authenticated user's name, email, or avatar URL.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body updateProfileRequest true "Fields to update"
// @Success 200 {object} userResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Security BearerAuth
// @Router /auth/me [patch]
func (h *AuthHandler) UpdateProfile(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	var req updateProfileRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid request body"})
		return
	}
	if req.Name == nil && req.Email == nil && req.AvatarURL == nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "no fields to update"})
		return
	}

	var current model.User
	if err := h.db.First(&current, "id = ?", claims.UserID).Error; err != nil {
		writeJSON(w, http.StatusNotFound, map[string]string{"error": "user not found"})
		return
	}

	updates := map[string]any{}
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "name cannot be empty"})
			return
		}
		updates["name"] = trimmed
	}
	if req.AvatarURL != nil {
		updates["avatar_url"] = strings.TrimSpace(*req.AvatarURL)
	}

	emailChanged := false
	if req.Email != nil {
		trimmed := strings.TrimSpace(*req.Email)
		if trimmed == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "email cannot be empty"})
			return
		}
		if !strings.EqualFold(trimmed, current.Email) {
			var existing model.User
			if err := h.db.Where("LOWER(email) = ? AND id != ?", strings.ToLower(trimmed), claims.UserID).First(&existing).Error; err == nil {
				writeJSON(w, http.StatusConflict, map[string]string{"error": "email already in use"})
				return
			}
			updates["email"] = trimmed
			// A self-service email change must always be re-verified (even with
			// HIVY_AUTO_CONFIRM_EMAIL), or a user could change to an allowlisted
			// platform-admin address and gain platform admin (derived from email).
			emailChanged = true
		}
	}

	if err := h.db.Model(&model.User{}).Where("id = ?", claims.UserID).Updates(updates).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to update profile", "user_id", claims.UserID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to update profile"})
		return
	}

	if emailChanged {
		h.db.Model(&model.User{}).Where("id = ?", claims.UserID).Update("email_confirmed_at", nil)
		var updated model.User
		if err := h.db.First(&updated, "id = ?", claims.UserID).Error; err == nil {
			if err := h.sendEmailConfirmationCode(r.Context(), updated); err != nil {
				logging.FromContext(r.Context()).ErrorContext(r.Context(), "send confirmation code after email change", "error", err)
			}
		}
	}

	var user model.User
	if err := h.db.First(&user, "id = ?", claims.UserID).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to reload user"})
		return
	}

	writeJSON(w, http.StatusOK, userResponse{
		ID:             user.ID.String(),
		Email:          user.Email,
		Name:           user.Name,
		AvatarURL:      user.AvatarURL,
		EmailConfirmed: user.EmailConfirmedAt != nil,
	})
}

// DeleteAccount handles DELETE /auth/me.
// @Summary Delete current user account
// @Description Permanently deletes the authenticated user's account. This action cannot be undone.
// @Tags auth
// @Produce json
// @Success 200 {object} statusResponse
// @Failure 401 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /auth/me [delete]
func (h *AuthHandler) DeleteAccount(w http.ResponseWriter, r *http.Request) {
	claims, ok := middleware.AuthClaimsFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return
	}

	if err := h.db.Where("id = ?", claims.UserID).Delete(&model.User{}).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to delete account", "user_id", claims.UserID, "error", err)
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "failed to delete account"})
		return
	}

	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
