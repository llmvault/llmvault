package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// Create handles POST /v1/channels.
// @Summary Create a channel
// @Description Creates a web-first channel. The creator becomes channel owner when the caller is a user.
// @Tags channels
// @Accept json
// @Produce json
// @Param body body channelMutationRequest true "Channel create payload"
// @Success 201 {object} channelMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels [post]
func (h *ChannelHandler) Create(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := orgForChannelRequest(w, ctx)
	if !ok {
		return
	}
	var req channelMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	source, ok := channelSourceFromCreate(w, &req)
	if !ok {
		return
	}
	name := channelNameFromCreate(&req, source)
	if name == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name is required"})
		return
	}
	if isReservedChannelName(name) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel name is reserved"})
		return
	}
	visibility := defaultString(cleanStringPtr(req.Visibility), "public")
	if !validChannelVisibility(visibility) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "visibility must be public or private"})
		return
	}
	defaultAgentID, ok := h.resolveDefaultAgentID(ctx, w, org.ID, req.DefaultAgentID)
	if !ok {
		return
	}
	imageModel := cleanStringPtr(req.ImageModel)
	if err := validateImageModelPreference(imageModel, false); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	vectorImageModel := cleanStringPtr(req.VectorImageModel)
	if err := validateImageModelPreference(vectorImageModel, true); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
		return
	}
	teamID, ok := h.resolveTeamID(ctx, w, org.ID, req.TeamID)
	if !ok {
		return
	}
	source, ok = h.prepareExternalChannelCreate(w, r, org.ID, source)
	if !ok {
		return
	}
	userID, hasUser := currentRequestUserID(ctx)
	channel := model.Channel{
		OrgID:                org.ID,
		Name:                 name,
		Description:          cleanStringPtr(req.Description),
		Kind:                 "standard",
		Visibility:           visibility,
		TeamID:               teamID,
		DefaultAgentID:       defaultAgentID,
		ImageModel:           imageModel,
		VectorImageModel:     vectorImageModel,
		Origin:               source.Origin,
		ExternalProvider:     source.ExternalProvider,
		ExternalConnectionID: source.ExternalConnectionID,
		ExternalWorkspaceKey: source.ExternalWorkspaceKey,
		ExternalResourceType: source.ExternalResourceType,
		ExternalResourceKey:  source.ExternalResourceKey,
		ExternalResourceName: source.ExternalResourceName,
		ExternalResourceURL:  source.ExternalResourceURL,
		ExternalMetadata:     source.ExternalMetadata,
	}
	if hasUser {
		channel.CreatedBy = userID
	}
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&channel).Error; err != nil {
			return err
		}
		if hasUser {
			return tx.Create(&model.ChannelMember{
				ChannelID: channel.ID,
				UserID:    *userID,
				Role:      "owner",
			}).Error
		}
		return nil
	})
	if err != nil {
		if isDuplicateKeyError(err) {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "channel already exists for this source"})
			return
		}
		logging.FromContext(ctx).ErrorContext(ctx, "create channel", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to create channel"})
		return
	}
	role := ""
	if hasUser {
		role = "owner"
	}
	writeJSON(w, http.StatusCreated, channelMutationResponse{
		Channel: channelToResponse(channel, role, h.memberCount(ctx, channel.ID)),
	})
}

// Update handles PATCH /v1/channels/{id}.
// @Summary Update a channel
// @Description Updates a channel when the caller is a channel owner, org admin, or scoped API key.
// @Tags channels
// @Accept json
// @Produce json
// @Param id path string true "Channel ID"
// @Param body body channelMutationRequest true "Channel update payload"
// @Success 200 {object} channelMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id} [patch]
func (h *ChannelHandler) Update(w http.ResponseWriter, r *http.Request) {
	channel, _, role, ok := h.authorizeChannel(w, r, true)
	if !ok {
		return
	}
	var req channelMutationRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	updates := map[string]any{}
	if !h.applyChannelUpdates(w, r, &channel, &req, updates) {
		return
	}
	if len(updates) > 0 {
		if err := h.db.WithContext(r.Context()).
			Model(&model.Channel{}).
			Where("id = ? AND org_id = ?", channel.ID, channel.OrgID).
			Updates(updates).Error; err != nil {
			if isDuplicateKeyError(err) {
				writeJSON(w, http.StatusConflict, errorResponse{Error: "channel already exists for this source"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to update channel"})
			return
		}
	}
	writeJSON(w, http.StatusOK, channelMutationResponse{
		Channel: channelToResponse(channel, role, h.memberCount(r.Context(), channel.ID)),
	})
}

// Archive handles DELETE /v1/channels/{id}.
// @Summary Archive a channel
// @Description Archives a non-default, non-personal channel.
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Success 200 {object} channelMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id} [delete]
func (h *ChannelHandler) Archive(w http.ResponseWriter, r *http.Request) {
	channel, _, role, ok := h.authorizeChannel(w, r, true)
	if !ok {
		return
	}
	if channel.IsDefault || channel.Kind == "personal" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default and personal channels cannot be archived"})
		return
	}
	now := time.Now()
	if err := h.db.WithContext(r.Context()).
		Model(&model.Channel{}).
		Where("id = ? AND org_id = ?", channel.ID, channel.OrgID).
		Update("archived_at", &now).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to archive channel"})
		return
	}
	channel.ArchivedAt = &now
	writeJSON(w, http.StatusOK, channelMutationResponse{
		Channel: channelToResponse(channel, role, h.memberCount(r.Context(), channel.ID)),
	})
}
