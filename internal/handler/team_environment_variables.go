package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// WithTeamEnvEncryptionKey wires the AES-256-GCM key used to encrypt and
// decrypt team environment variable values.
func WithTeamEnvEncryptionKey(key *crypto.SymmetricKey) TeamHandlerOption {
	return func(h *TeamHandler) {
		h.envEncKey = key
	}
}

// ListEnvironmentVariables handles GET /v1/orgs/current/teams/{id}/environment-variables.
// @Summary List team environment variables
// @Description Lists environment variables shared by all channels in a team. Values are not returned.
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Success 200 {object} teamEnvironmentVariablesResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id}/environment-variables [get]
func (h *TeamHandler) ListEnvironmentVariables(w http.ResponseWriter, r *http.Request) {
	team, ok := h.authorizeEnvironmentVariableTeam(w, r)
	if !ok {
		return
	}
	var vars []model.TeamEnvVar
	if err := h.db.WithContext(r.Context()).
		Where("org_id = ? AND team_id = ?", team.OrgID, team.ID).
		Order("name").
		Find(&vars).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to list team environment variables", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load environment variables"})
		return
	}
	data := make([]teamEnvironmentVariableResponse, len(vars))
	for i, envVar := range vars {
		data[i] = teamEnvironmentVariableResponse{Name: envVar.Name, Description: envVar.Description}
	}
	writeJSON(w, http.StatusOK, teamEnvironmentVariablesResponse{Data: data})
}

// CreateEnvironmentVariable handles POST /v1/orgs/current/teams/{id}/environment-variables.
// @Summary Create a team environment variable
// @Description Stores an environment variable shared by all channels in a team.
// @Tags teams
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param body body createTeamEnvironmentVariableRequest true "Environment variable"
// @Success 201 {object} teamEnvironmentVariableResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id}/environment-variables [post]
func (h *TeamHandler) CreateEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	var req createTeamEnvironmentVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	name, err := normalizeTeamEnvName(req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if req.Value == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "value is required"})
		return
	}
	description := strings.TrimSpace(req.Description)
	if len(description) > maxTeamEnvDescriptionLen {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "description is too long"})
		return
	}
	team, ok := h.authorizeEnvironmentVariableTeam(w, r)
	if !ok || !h.envEncryptionConfigured(w) {
		return
	}
	encrypted, err := h.envEncKey.EncryptString(req.Value)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to encrypt team environment variable", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save environment variable"})
		return
	}
	envVar := model.TeamEnvVar{OrgID: team.OrgID, TeamID: team.ID, Name: name, EncryptedValue: encrypted, Description: description}
	if err := h.db.WithContext(r.Context()).Create(&envVar).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "environment variable already exists"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to create team environment variable", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save environment variable"})
		return
	}
	writeJSON(w, http.StatusCreated, teamEnvironmentVariableResponse{Name: name, Description: description})
}

// UpdateEnvironmentVariable handles PATCH /v1/orgs/current/teams/{id}/environment-variables/{name}.
// @Summary Update a team environment variable
// @Description Renames and/or updates a team environment variable.
// @Tags teams
// @Accept json
// @Produce json
// @Param id path string true "Team ID"
// @Param name path string true "Environment variable name"
// @Param body body updateTeamEnvironmentVariableRequest true "Fields to patch"
// @Success 200 {object} teamEnvironmentVariableResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id}/environment-variables/{name} [patch]
func (h *TeamHandler) UpdateEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	currentName, err := normalizeTeamEnvName(chi.URLParam(r, "name"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	var req updateTeamEnvironmentVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if req.Name == nil && req.Value == nil && req.Description == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "no fields to update"})
		return
	}
	if req.Value != nil && *req.Value == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "value cannot be empty"})
		return
	}
	if req.Description != nil && len(strings.TrimSpace(*req.Description)) > maxTeamEnvDescriptionLen {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "description is too long"})
		return
	}
	newName := currentName
	if req.Name != nil {
		newName, err = normalizeTeamEnvName(*req.Name)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
	}
	team, ok := h.authorizeEnvironmentVariableTeam(w, r)
	if !ok || !h.envEncryptionConfigured(w) {
		return
	}
	var envVar model.TeamEnvVar
	if err := h.db.WithContext(r.Context()).
		Where("org_id = ? AND team_id = ? AND name = ?", team.OrgID, team.ID, currentName).
		First(&envVar).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "environment variable not found"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to load team environment variable", "error", err, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load environment variable"})
		return
	}
	description := envVar.Description
	updates := map[string]any{}
	if newName != currentName {
		updates["name"] = newName
	}
	if req.Value != nil {
		encrypted, err := h.envEncKey.EncryptString(*req.Value)
		if err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to encrypt team environment variable", "error", err, "team_id", team.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save environment variable"})
			return
		}
		updates["encrypted_value"] = encrypted
	}
	if req.Description != nil {
		description = strings.TrimSpace(*req.Description)
		updates["description"] = description
	}
	if len(updates) > 0 {
		result := h.db.WithContext(r.Context()).Model(&model.TeamEnvVar{}).
			Where("id = ? AND org_id = ? AND team_id = ?", envVar.ID, team.OrgID, team.ID).
			Updates(updates)
		if result.Error != nil {
			if isDuplicateKeyError(result.Error) {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "environment variable already exists"})
				return
			}
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to update team environment variable", "error", result.Error, "team_id", team.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save environment variable"})
			return
		}
		if result.RowsAffected != 1 {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "environment variable not found"})
			return
		}
	}
	writeJSON(w, http.StatusOK, teamEnvironmentVariableResponse{Name: newName, Description: description})
}

// DeleteEnvironmentVariable handles DELETE /v1/orgs/current/teams/{id}/environment-variables/{name}.
// @Summary Delete a team environment variable
// @Description Removes an environment variable shared by a team.
// @Tags teams
// @Produce json
// @Param id path string true "Team ID"
// @Param name path string true "Environment variable name"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/teams/{id}/environment-variables/{name} [delete]
func (h *TeamHandler) DeleteEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	name, err := normalizeTeamEnvName(chi.URLParam(r, "name"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	team, ok := h.authorizeEnvironmentVariableTeam(w, r)
	if !ok {
		return
	}
	result := h.db.WithContext(r.Context()).
		Where("org_id = ? AND team_id = ? AND name = ?", team.OrgID, team.ID, name).
		Delete(&model.TeamEnvVar{})
	if result.Error != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to delete team environment variable", "error", result.Error, "team_id", team.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete environment variable"})
		return
	}
	if result.RowsAffected != 1 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "environment variable not found"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "deleted"})
}

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
