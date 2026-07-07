package handler

import (
	"encoding/json"
	"errors"
	"net/http"
	"sort"
	"strings"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// WithChannelEnvEncryptionKey wires the AES-256-GCM key used to encrypt and
// decrypt per-channel environment variable values.
func WithChannelEnvEncryptionKey(key *crypto.SymmetricKey) ChannelHandlerOption {
	return func(h *ChannelHandler) {
		h.envEncKey = key
	}
}

// WithChannelEnqueuer wires the async task enqueuer used to schedule
// post-delete cleanup (channel memory deletion).
func WithChannelEnqueuer(enq enqueue.TaskEnqueuer) ChannelHandlerOption {
	return func(h *ChannelHandler) {
		h.enqueuer = enq
	}
}

// ListChannelEnvironmentVariables handles GET /v1/channels/{id}/environment-variables.
// @Summary List channel environment variables
// @Description Lists environment variables scoped to a channel. Values are not returned.
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Success 200 {object} channelEnvironmentVariablesResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/environment-variables [get]
func (h *ChannelHandler) ListChannelEnvironmentVariables(w http.ResponseWriter, r *http.Request) {
	channel, _, _, ok := h.authorizeChannel(w, r, false)
	if !ok {
		return
	}

	var vars []model.ChannelEnvVar
	if err := h.db.WithContext(r.Context()).
		Where("channel_id = ?", channel.ID).
		Order("name").
		Find(&vars).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to list channel environment variables", "error", err, "channel_id", channel.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load environment variables"})
		return
	}

	data := make([]channelEnvironmentVariableResponse, 0, len(vars))
	for _, v := range vars {
		data = append(data, channelEnvironmentVariableResponse{Name: v.Name, Description: v.Description})
	}
	sort.Slice(data, func(i, j int) bool { return data[i].Name < data[j].Name })
	writeJSON(w, http.StatusOK, channelEnvironmentVariablesResponse{Data: data})
}

// CreateChannelEnvironmentVariable handles POST /v1/channels/{id}/environment-variables.
// @Summary Create a channel environment variable
// @Description Stores an environment variable scoped to a channel. It is injected into sessions on that channel as __ENV__<NAME> and exposed to the workload as <NAME>.
// @Tags channels
// @Accept json
// @Produce json
// @Param id path string true "Channel ID"
// @Param body body createChannelEnvironmentVariableRequest true "Environment variable"
// @Success 201 {object} channelEnvironmentVariableResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/environment-variables [post]
func (h *ChannelHandler) CreateChannelEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	channel, _, _, ok := h.authorizeChannel(w, r, true)
	if !ok {
		return
	}
	if !h.envEncryptionConfigured(w) {
		return
	}

	var req createChannelEnvironmentVariableRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	name, err := normalizeChannelEnvName(req.Name)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	if req.Value == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "value is required"})
		return
	}

	encrypted, err := h.envEncKey.EncryptString(req.Value)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to encrypt channel environment variable", "error", err, "channel_id", channel.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save environment variable"})
		return
	}

	description := strings.TrimSpace(req.Description)
	if len(description) > maxChannelEnvDescriptionLen {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "description is too long"})
		return
	}
	envVar := model.ChannelEnvVar{
		OrgID:          channel.OrgID,
		ChannelID:      channel.ID,
		Name:           name,
		EncryptedValue: encrypted,
		Description:    description,
	}
	if err := h.db.WithContext(r.Context()).Create(&envVar).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "environment variable already exists"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to create channel environment variable", "error", err, "channel_id", channel.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save environment variable"})
		return
	}
	writeJSON(w, http.StatusCreated, channelEnvironmentVariableResponse{Name: name, Description: description})
}

// UpdateChannelEnvironmentVariable handles PATCH /v1/channels/{id}/environment-variables/{name}.
// @Summary Update a channel environment variable
// @Description Renames and/or updates the value of a channel environment variable.
// @Tags channels
// @Accept json
// @Produce json
// @Param id path string true "Channel ID"
// @Param name path string true "Environment variable name"
// @Param body body updateChannelEnvironmentVariableRequest true "Fields to patch"
// @Success 200 {object} channelEnvironmentVariableResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/environment-variables/{name} [patch]
func (h *ChannelHandler) UpdateChannelEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	channel, _, _, ok := h.authorizeChannel(w, r, true)
	if !ok {
		return
	}
	if !h.envEncryptionConfigured(w) {
		return
	}

	currentName, err := normalizeChannelEnvName(chi.URLParam(r, "name"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	var req updateChannelEnvironmentVariableRequest
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
	if req.Description != nil && len(strings.TrimSpace(*req.Description)) > maxChannelEnvDescriptionLen {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "description is too long"})
		return
	}

	newName := currentName
	if req.Name != nil {
		newName, err = normalizeChannelEnvName(*req.Name)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return
		}
	}

	var envVar model.ChannelEnvVar
	if err := h.db.WithContext(r.Context()).
		Where("channel_id = ? AND name = ?", channel.ID, currentName).
		First(&envVar).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "environment variable not found"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to load channel environment variable", "error", err, "channel_id", channel.ID)
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
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to encrypt channel environment variable", "error", err, "channel_id", channel.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save environment variable"})
			return
		}
		updates["encrypted_value"] = encrypted
	}
	if req.Description != nil {
		description = strings.TrimSpace(*req.Description)
		updates["description"] = description
	}
	if len(updates) == 0 {
		writeJSON(w, http.StatusOK, channelEnvironmentVariableResponse{Name: newName, Description: description})
		return
	}

	if err := h.db.WithContext(r.Context()).
		Model(&model.ChannelEnvVar{}).
		Where("id = ?", envVar.ID).
		Updates(updates).Error; err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "environment variable already exists"})
			return
		}
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to update channel environment variable", "error", err, "channel_id", channel.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save environment variable"})
		return
	}
	writeJSON(w, http.StatusOK, channelEnvironmentVariableResponse{Name: newName, Description: description})
}

// DeleteChannelEnvironmentVariable handles DELETE /v1/channels/{id}/environment-variables/{name}.
// @Summary Delete a channel environment variable
// @Description Removes an environment variable scoped to a channel.
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Param name path string true "Environment variable name"
// @Success 200 {object} statusResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/environment-variables/{name} [delete]
func (h *ChannelHandler) DeleteChannelEnvironmentVariable(w http.ResponseWriter, r *http.Request) {
	channel, _, _, ok := h.authorizeChannel(w, r, true)
	if !ok {
		return
	}

	name, err := normalizeChannelEnvName(chi.URLParam(r, "name"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}

	result := h.db.WithContext(r.Context()).
		Where("channel_id = ? AND name = ?", channel.ID, name).
		Delete(&model.ChannelEnvVar{})
	if result.Error != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to delete channel environment variable", "error", result.Error, "channel_id", channel.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete environment variable"})
		return
	}
	if result.RowsAffected == 0 {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "environment variable not found"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "deleted"})
}

func (h *ChannelHandler) envEncryptionConfigured(w http.ResponseWriter) bool {
	if h.envEncKey == nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "environment variable encryption is not configured"})
		return false
	}
	return true
}
