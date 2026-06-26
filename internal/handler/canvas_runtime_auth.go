package handler

import (
	"context"
	"crypto/subtle"
	"errors"
	"fmt"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	canvaspkg "github.com/usehivy/hivy/internal/canvas"
	"github.com/usehivy/hivy/internal/canvasartifact"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (h *CanvasHandler) authorizeRuntimeRequest(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	if h == nil || h.db == nil || h.encKey == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "runtime authentication is not configured"})
		return uuid.Nil, false
	}
	agentID, err := uuid.Parse(chi.URLParam(r, "agentID"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid agent_id"})
		return uuid.Nil, false
	}
	token := extractBearerToken(r)
	if token == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing authorization"})
		return uuid.Nil, false
	}
	if err := h.verifyRuntimeSecret(r.Context(), agentID, token); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "agent not found"})
			return uuid.Nil, false
		}
		if errors.Is(err, errInvalidRuntimeSecret) {
			writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid credentials"})
			return uuid.Nil, false
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "canvas runtime auth", "error", err, "agent_id", agentID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to verify credentials"})
		return uuid.Nil, false
	}
	return agentID, true
}

var errInvalidRuntimeSecret = errors.New("invalid runtime secret")

func (h *CanvasHandler) verifyRuntimeSecret(ctx context.Context, agentID uuid.UUID, bearerToken string) error {
	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ?", agentID).First(&agent).Error; err != nil {
		return err
	}
	var sandboxes []model.Sandbox
	if err := h.db.WithContext(ctx).Where("agent_id = ?", agentID).Find(&sandboxes).Error; err != nil {
		return fmt.Errorf("load sandboxes: %w", err)
	}
	for _, sb := range sandboxes {
		decrypted, err := h.encKey.DecryptString(sb.EncryptedRuntimeSecret)
		if err != nil {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(bearerToken), []byte(decrypted)) == 1 {
			return nil
		}
	}
	return errInvalidRuntimeSecret
}

func (h *CanvasHandler) writeRuntimeCanvasError(w http.ResponseWriter, r *http.Request, err error, op string, agentID uuid.UUID) {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "canvas resource not found"})
		return
	}
	if errors.Is(err, canvaspkg.ErrNotConfigured) {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas is not configured"})
		return
	}
	if errors.Is(err, canvasartifact.ErrStorageNotConfigured) {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "canvas artifact storage is not configured"})
		return
	}
	logging.FromContext(r.Context()).ErrorContext(r.Context(), "canvas runtime request failed", "operation", op, "error", err, "agent_id", agentID)
	writeJSON(w, http.StatusBadGateway, errorResponse{Error: "canvas request failed"})
}
