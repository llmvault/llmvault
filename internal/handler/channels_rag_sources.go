package handler

import (
	"encoding/json"
	"net/http"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

type channelRAGSourceResponse struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Kind   string `json:"kind"`
	Status string `json:"status"`
}

type channelRAGSourcesResponse struct {
	Data []channelRAGSourceResponse `json:"data"`
}

type setChannelRAGSourcesRequest struct {
	SourceIDs []string `json:"source_ids"`
}

// ListChannelRAGSources handles GET /v1/channels/{id}/rag-sources.
// @Summary List a channel's knowledge sources
// @Description Lists the RAG sources a channel is granted access to. Agents in the channel can only search these sources.
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

	var sources []ragmodel.RAGSource
	if err := h.db.WithContext(r.Context()).
		Joins("JOIN channel_rag_sources ON channel_rag_sources.rag_source_id = rag_sources.id").
		Where("channel_rag_sources.channel_id = ?", channel.ID).
		Order("rag_sources.name").
		Find(&sources).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to list channel rag sources", "error", err, "channel_id", channel.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load knowledge sources"})
		return
	}

	writeJSON(w, http.StatusOK, channelRAGSourcesResponse{Data: toChannelRAGSourceResponses(sources)})
}

// SetChannelRAGSources handles PUT /v1/channels/{id}/rag-sources.
// @Summary Set a channel's knowledge sources
// @Description Replaces the full set of RAG sources a channel can search. Each source must belong to the org. An empty set removes all knowledge access.
// @Tags channels
// @Accept json
// @Produce json
// @Param id path string true "Channel ID"
// @Param body body setChannelRAGSourcesRequest true "Source IDs"
// @Success 200 {object} channelRAGSourcesResponse
// @Failure 400 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/channels/{id}/rag-sources [put]
func (h *ChannelHandler) SetChannelRAGSources(w http.ResponseWriter, r *http.Request) {
	channel, _, _, ok := h.authorizeChannel(w, r, true)
	if !ok {
		return
	}

	var req setChannelRAGSourcesRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	// Dedupe and parse the requested source IDs.
	seen := make(map[uuid.UUID]struct{}, len(req.SourceIDs))
	ids := make([]uuid.UUID, 0, len(req.SourceIDs))
	for _, raw := range req.SourceIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid source id: " + raw})
			return
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	// Every requested source must exist and belong to this org.
	if len(ids) > 0 {
		var count int64
		if err := h.db.WithContext(r.Context()).
			Model(&ragmodel.RAGSource{}).
			Where("id IN ? AND org_id = ?", ids, channel.OrgID).
			Count(&count).Error; err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to validate rag sources", "error", err, "channel_id", channel.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save knowledge sources"})
			return
		}
		if int(count) != len(ids) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "one or more sources do not exist in this org"})
			return
		}
	}

	// Replace the full set atomically.
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("channel_id = ?", channel.ID).Delete(&model.ChannelRagSource{}).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		rows := make([]model.ChannelRagSource, 0, len(ids))
		for _, id := range ids {
			rows = append(rows, model.ChannelRagSource{
				OrgID:       channel.OrgID,
				ChannelID:   channel.ID,
				RagSourceID: id,
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to set channel rag sources", "error", err, "channel_id", channel.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save knowledge sources"})
		return
	}

	var sources []ragmodel.RAGSource
	if len(ids) > 0 {
		if err := h.db.WithContext(r.Context()).
			Where("id IN ? AND org_id = ?", ids, channel.OrgID).
			Order("name").
			Find(&sources).Error; err != nil {
			logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to reload channel rag sources", "error", err, "channel_id", channel.ID)
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save knowledge sources"})
			return
		}
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
