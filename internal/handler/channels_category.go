package handler

import (
	"net/http"
	"strings"

	"github.com/usehivy/hivy/internal/memory"
)

const channelCategoryError = "category must be one of: customer-support, account, engineering, operations, sales, marketing, people, general"

// channelCategoryFromCreate resolves the required category for a new channel.
// Native channels must pick one (the create UI shows the picker); external
// auto-provisioned channels (Slack etc.) have no picker and default 'general'.
func channelCategoryFromCreate(w http.ResponseWriter, req *channelMutationRequest, source channelSourceFields) (string, bool) {
	category := strings.ToLower(cleanStringPtr(req.Category))
	if category == "" {
		if source.Origin == "external" {
			return memory.ChannelCategoryGeneral, true
		}
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "category is required"})
		return "", false
	}
	if !memory.ValidChannelCategory(category) {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: channelCategoryError})
		return "", false
	}
	return category, true
}

// channelMissionForCreate seeds the initial mission: an explicit request value
// wins; otherwise the category's curated template is stored verbatim
// ('general' => no mission). Fully deterministic — no LLM specialization.
func channelMissionForCreate(req *channelMutationRequest, category string) *string {
	if req.MemoryMission != nil {
		if mission := strings.TrimSpace(*req.MemoryMission); mission != "" {
			return &mission
		}
		return nil
	}
	if template := memory.MissionTemplate(category); template != "" {
		return &template
	}
	return nil
}
