package handler

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// @Summary Register
// @Description Creates a user account with email and password.
// @Tags auth
// @Accept json
// @Produce json
// @Param body body registerRequest true "Registration parameters"
// @Success 201 {object} authResponse
// @Failure 400 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Router /auth/register [post]
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var req registerRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.Name = strings.TrimSpace(req.Name)

	if req.Email == "" || req.Password == "" || req.Name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "email, password, and name are required"})
		return
	}
	if len(req.Password) < 8 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "password must be at least 8 characters"})
		return
	}

	// Check if email is taken.
	var existing model.User
	err := h.db.WithContext(ctx).Where("email = ?", req.Email).First(&existing).Error
	if err == nil {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "email already registered"})
		return
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		logging.FromContext(ctx).ErrorContext(ctx, "check registration email", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create account"})
		return
	}

	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to hash password", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "internal error"})
		return
	}

	// Create user, org, membership, and any welcome credit grant in a single
	// transaction so signup is all-or-nothing.
	var user model.User
	var org model.Org

	err = h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		user = model.User{
			Email:        req.Email,
			PasswordHash: hash,
			Name:         req.Name,
		}
		if err := tx.Create(&user).Error; err != nil {
			return fmt.Errorf("creating user: %w", err)
		}

		var orgErr error
		org, orgErr = createUserDefaultOrg(ctx, tx, h.credits, &user)
		return orgErr
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "email already registered"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to register user", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create account"})
		return
	}
	if h.autoConfirmEmail {

		now := time.Now()
		result := h.db.WithContext(ctx).Model(&model.User{}).
			Where("id = ? AND email_confirmed_at IS NULL", user.ID).
			Update("email_confirmed_at", &now)
		if result.Error != nil || result.RowsAffected != 1 {
			if result.Error != nil {
				logging.FromContext(ctx).ErrorContext(ctx, "auto-confirm registered email", "error", result.Error, "user_id", user.ID)
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create account"})
			return
		}
		user.EmailConfirmedAt = &now
	} else {

		if err := h.sendEmailConfirmationCode(r.Context(), user); err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to send confirmation code", "error", err, "user_id", user.ID)
		}
	}

	logging.FromContext(r.Context()).InfoContext(r.Context(), "user registered", "user_id", user.ID, "email", user.Email)
	h.issueTokensAndRespond(r.Context(), w, http.StatusCreated, user, org.ID.String(), "owner")
}
