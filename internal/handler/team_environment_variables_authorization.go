package handler

import (
	"errors"
	"net/http"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

func (h *TeamHandler) authorizeEnvironmentVariableTeam(w http.ResponseWriter, r *http.Request) (model.Team, bool) {
	org, ok := orgForTeamRequest(w, r)
	if !ok {
		return model.Team{}, false
	}
	teamID, ok := teamIDFromRequest(w, r)
	if !ok {
		return model.Team{}, false
	}
	var team model.Team
	err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", teamID, org.ID).
		First(&team).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return model.Team{}, false
	}
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to load team for environment variables", "error", err, "team_id", teamID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load team"})
		return model.Team{}, false
	}
	if isAPIKeyRequest(r.Context()) {
		return team, true
	}
	rawUserID := middleware.UserID(r.Context())
	if strings.TrimSpace(rawUserID) == "" {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing auth context"})
		return model.Team{}, false
	}
	actor, err := access.Resolve(r.Context(), h.db, org.ID, rawUserID)
	if err != nil || actor == nil {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return model.Team{}, false
	}
	allowed, err := actor.CanManageTeamResource(r.Context(), h.db, team.ID)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to authorize team environment variables", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to authorize team"})
		return model.Team{}, false
	}
	if !allowed {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "team not found"})
		return model.Team{}, false
	}
	return team, true
}

func (h *TeamHandler) envEncryptionConfigured(w http.ResponseWriter) bool {
	if h.envEncKey == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "environment variable encryption is not configured"})
		return false
	}
	return true
}
