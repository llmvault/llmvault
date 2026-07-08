package handler

import (
	"net/http"

	"github.com/usehivy/hivy/internal/logging"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

// Knowledge (RAG) grants are TEAM-level now (team_rag_sources), provisioned by
// org admins via /v1/orgs/current/teams/{teamID}/rag-sources. The old
// channel-scoped grant path (PUT /channels/{id}/rag-sources) was an escalation
// hole — any channel owner/manager could widen their own knowledge reach — and
// has been removed. This file keeps only the READ, derived from the channel's
// team grants: the sources a channel's agents may search are exactly the
// sources granted to the channel's team.

type channelRAGSourceResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type channelRAGSourcesResponse struct {
	Data []channelRAGSourceResponse `json:"data"`
}

// ListChannelRAGSources handles GET /v1/channels/{id}/rag-sources.
// @Summary List a channel's knowledge sources
// @Description Lists the RAG sources a channel's agents can search. These are derived from the grants of the channel's team; a channel with no team has no knowledge access. Grants are managed at the team level by org admins.
// @Tags channels
// @Produce json
// @Param id path string true "Channel ID"
// @Success 200 {object} channelRAGSourcesResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/rag-sources [get]
func (h *ChannelHandler) ListChannelRAGSources(w http.ResponseWriter, r *http.Request) {
	channel, _, _, ok := h.authorizeChannel(w, r, false)
	if !ok {
		return
	}

	// No team → no team grants → no knowledge access.
	if channel.TeamID == nil {
		writeJSON(w, http.StatusOK, channelRAGSourcesResponse{Data: []channelRAGSourceResponse{}})
		return
	}

	var sources []ragmodel.RAGSource
	if err := h.db.WithContext(r.Context()).
		Joins("JOIN team_rag_sources ON team_rag_sources.rag_source_id = rag_sources.id").
		Where("team_rag_sources.team_id = ? AND team_rag_sources.org_id = ?", *channel.TeamID, channel.OrgID).
		Order("rag_sources.name").
		Find(&sources).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to list channel rag sources", "error", err, "channel_id", channel.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load knowledge sources"})
		return
	}

	writeJSON(w, http.StatusOK, channelRAGSourcesResponse{Data: toChannelRAGSourceResponses(sources)})
}

func toChannelRAGSourceResponses(sources []ragmodel.RAGSource) []channelRAGSourceResponse {
	data := make([]channelRAGSourceResponse, 0, len(sources))
	for _, s := range sources {
		data = append(data, channelRAGSourceResponse{
			ID:     s.ID.String(),
			Name:   s.Name,
			Kind:   string(s.KindValue),
			Status: string(s.Status),
		})
	}
	return data
}
