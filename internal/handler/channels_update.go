package handler

import (
	"net/http"
	"strings"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

func (h *ChannelHandler) applyChannelUpdates(w http.ResponseWriter, r *http.Request, channel *model.Channel, req *channelMutationRequest, updates map[string]any) bool {
	if req.Name != nil {
		name := normalizeChannelName(*req.Name)
		if name == "" {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "name cannot be empty"})
			return false
		}
		if isReservedChannelName(name) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "channel name is reserved"})
			return false
		}
		updates["name"] = name
		channel.Name = name
	}
	if req.Description != nil {
		value := cleanStringPtr(req.Description)
		updates["description"] = value
		channel.Description = value
	}
	if req.Category != nil {
		value := strings.ToLower(cleanStringPtr(req.Category))
		if !memory.ValidChannelCategory(value) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: channelCategoryError})
			return false
		}
		updates["category"] = value
		channel.Category = value
	}
	if req.MemoryMission != nil {
		value := strings.TrimSpace(*req.MemoryMission)
		if value == "" {
			updates["memory_mission"] = nil
			channel.MemoryMission = nil
		} else {
			updates["memory_mission"] = value
			channel.MemoryMission = &value
		}
	}
	if req.Visibility != nil {
		value := cleanStringPtr(req.Visibility)
		if !validChannelVisibility(value) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "visibility must be public or private"})
			return false
		}
		updates["visibility"] = value
		channel.Visibility = value
	}
	if req.TeamID != nil {
		teamID, ok := h.resolveTeamID(r.Context(), w, channel.OrgID, req.TeamID)
		if !ok {
			return false
		}
		updates["team_id"] = teamID
		channel.TeamID = teamID
	}
	if req.DefaultAgentID != nil {
		agentID, ok := h.resolveDefaultAgentID(r.Context(), w, channel.OrgID, channel.TeamID, req.DefaultAgentID)
		if !ok {
			return false
		}
		updates["default_agent_id"] = agentID
		channel.DefaultAgentID = agentID
	}
	if req.ImageModel != nil {
		value := cleanStringPtr(req.ImageModel)
		if err := validateImageModelPreference(value, false); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return false
		}
		updates["image_model"] = value
		channel.ImageModel = value
	}
	if req.VectorImageModel != nil {
		value := cleanStringPtr(req.VectorImageModel)
		if err := validateImageModelPreference(value, true); err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: err.Error()})
			return false
		}
		updates["vector_image_model"] = value
		channel.VectorImageModel = value
	}
	return applyChannelSourceUpdate(w, channel, req, updates)
}

func validChannelVisibility(value string) bool {
	return value == "public" || value == "private"
}
