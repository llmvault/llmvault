package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
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
	visibility := defaultString(cleanStringPtr(req.Visibility), "public")
	if !validChannelVisibility(visibility) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "visibility must be public or private"})
		return
	}
	category, ok := channelCategoryFromCreate(w, &req, source)
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
	if teamID == nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "team_id is required"})
		return
	}
	if !h.canCreateChannelInTeam(ctx, org.ID, teamID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "you must be a member of the target team to create a channel in it"})
		return
	}
	// Resolve the default agent after the team so it can be gated to the channel's
	// team (a team-scoped channel's default agent must belong to the same team).
	defaultAgentID, ok := h.resolveDefaultAgentID(ctx, w, org.ID, teamID, req.DefaultAgentID)
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
		Category:             category,
		MemoryMission:        channelMissionForCreate(&req, category),
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
		// No assignment row: under the team-primary model the default agent is
		// usable by virtue of belonging to the channel's team (resolveDefaultAgentID
		// enforces the same-team gate).
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
		// The new default agent is gated to the channel's team by
		// resolveDefaultAgentID; no assignment row is needed anymore.
		if err := h.db.WithContext(r.Context()).Model(&model.Channel{}).
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
// @Summary Delete a channel
// @Description Deletes a non-default, non-personal channel: it archives the channel and all its sessions, then queues deletion of the channel's memories. Rejected with 409 if any session has an in-progress agent turn.
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Success 200 {object} channelMutationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 409 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id} [delete]
func (h *ChannelHandler) Archive(w http.ResponseWriter, r *http.Request) {
	channel, _, role, ok := h.authorizeChannel(w, r, true)
	if !ok {
		return
	}
	if channel.IsDefault || channel.Kind == "personal" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "default and personal channels cannot be deleted"})
		return
	}
	ctx := r.Context()

	// A team must keep at least one channel — this protects the team's floor
	// (its #general) even if the default flag is ever cleared.
	if ok, err := h.isTeamLastChannel(ctx, channel); err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to check team channels"})
		return
	} else if ok {
		writeJSON(w, http.StatusConflict, errorResponse{Error: "a team must keep at least one channel"})
		return
	}

	// Load the channel's live (non-archived) sessions. Deleting tears their
	// sandboxes down, which would abort a live turn, so refuse while any turn is
	// in progress — the same idle rule single-session archive enforces.
	var sessions []model.Session
	if err := h.db.WithContext(ctx).
		Where("channel_id = ? AND org_id = ? AND status <> ?", channel.ID, channel.OrgID, "archived").
		Find(&sessions).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load channel sessions"})
		return
	}
	for _, session := range sessions {
		if session.AgentTurnStatus == model.SessionAgentTurnActive {
			writeJSON(w, http.StatusConflict, errorResponse{Error: "channel has a session with an in-progress turn; wait for it to finish, then try again"})
			return
		}
	}

	now := time.Now()
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.Channel{}).
			Where("id = ? AND org_id = ?", channel.ID, channel.OrgID).
			Update("archived_at", &now).Error; err != nil {
			return err
		}
		if len(sessions) == 0 {
			return nil
		}
		return tx.Model(&model.Session{}).
			Where("channel_id = ? AND org_id = ? AND status <> ?", channel.ID, channel.OrgID, "archived").
			Updates(map[string]any{
				"status":   "archived",
				"ended_at": gorm.Expr("COALESCE(ended_at, ?)", now),
			}).Error
	})
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete channel"})
		return
	}

	// Tear down each archived session's sandbox and queue removal of the
	// channel's memories. Best effort: the channel is already deleted, so a
	// failed enqueue is logged, not surfaced.
	for i := range sessions {
		if sessions[i].SandboxID == nil {
			continue
		}
		if err := tasks.EnqueueSandboxDelete(ctx, h.enqueuer, *sessions[i].SandboxID); err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "enqueue sandbox teardown on channel delete", "session_id", sessions[i].ID, "error", err)
		}
	}
	if err := tasks.EnqueueChannelMemoriesDelete(ctx, h.enqueuer, channel.OrgID, channel.ID); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "enqueue channel memories delete", "channel_id", channel.ID, "error", err)
	}

	channel.ArchivedAt = &now
	writeJSON(w, http.StatusOK, channelMutationResponse{
		Channel: channelToResponse(channel, role, h.memberCount(ctx, channel.ID)),
	})
}
