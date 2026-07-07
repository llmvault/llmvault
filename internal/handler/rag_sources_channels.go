package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	ragdb "github.com/usehivy/hivy/internal/rag/db"
)

type setSourceChannelsRequest struct {
	ChannelIDs []string `json:"channel_ids"`
}

type sourceChannelsResponse struct {
	ChannelIDs []string `json:"channel_ids"`
}

// GetSourceChannels handles GET /v1/rag/sources/{id}/channels.
// It returns the channels currently granted access to this source.
// @Summary List which channels can search a source
// @Description Returns the ids of the channels granted access to this knowledge source.
// @Tags rag
// @Produce json
// @Param id path string true "RAG source ID"
// @Success 200 {object} sourceChannelsResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/channels [get]
func (h *RAGSourceHandler) GetSourceChannels(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	srcID, ok := parseSourceID(chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid source id"})
		return
	}
	if _, err := ragdb.GetSourceForOrg(h.db, org.ID, srcID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "source not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load source"})
		return
	}

	// Non-manager members only learn about grants to channels they can use;
	// managers and API-key callers see every grant.
	orgWide, userID, err := actorSeesOrgWide(r.Context(), h.db, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	q := h.db.WithContext(r.Context()).
		Where("channel_rag_sources.rag_source_id = ? AND channel_rag_sources.org_id = ?", srcID, org.ID)
	if !orgWide {
		q = q.Joins("JOIN channels ON channels.id = channel_rag_sources.channel_id AND channels.archived_at IS NULL").
			Where("channels.origin = ? OR channels.team_id IS NULL OR channels.team_id IN (?)",
				"external", visibleTeamSubquery(h.db, userID))
	}
	var rows []model.ChannelRagSource
	if err := q.Find(&rows).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load channels"})
		return
	}
	out := make([]string, 0, len(rows))
	for _, row := range rows {
		out = append(out, row.ChannelID.String())
	}
	writeJSON(w, http.StatusOK, sourceChannelsResponse{ChannelIDs: out})
}

// SetSourceChannels handles PUT /v1/rag/sources/{id}/channels.
// It replaces the full set of channels that can search this source — the
// source-side counterpart to PUT /v1/channels/{id}/rag-sources. An empty set
// removes the source from every channel.
// @Summary Set which channels can search a source
// @Description Replaces the full set of channels granted access to this knowledge source. Each channel must belong to the org. An empty set removes all grants.
// @Tags rag
// @Accept json
// @Produce json
// @Param id path string true "RAG source ID"
// @Param body body setSourceChannelsRequest true "Channel IDs"
// @Success 200 {object} sourceChannelsResponse
// @Failure 400 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/channels [put]
func (h *RAGSourceHandler) SetSourceChannels(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	srcID, ok := parseSourceID(chi.URLParam(r, "id"))
	if !ok {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid source id"})
		return
	}

	// The source must exist in this org.
	if _, err := ragdb.GetSourceForOrg(h.db, org.ID, srcID); err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "source not found"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load source"})
		return
	}

	var req setSourceChannelsRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}

	seen := make(map[uuid.UUID]struct{}, len(req.ChannelIDs))
	ids := make([]uuid.UUID, 0, len(req.ChannelIDs))
	for _, raw := range req.ChannelIDs {
		id, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid channel id: " + raw})
			return
		}
		if _, dup := seen[id]; dup {
			continue
		}
		seen[id] = struct{}{}
		ids = append(ids, id)
	}

	// Every requested channel must belong to this org.
	if len(ids) > 0 {
		var count int64
		if err := h.db.WithContext(r.Context()).
			Model(&model.Channel{}).
			Where("id IN ? AND org_id = ?", ids, org.ID).
			Count(&count).Error; err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to validate channels"})
			return
		}
		if int(count) != len(ids) {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "one or more channels do not exist in this org"})
			return
		}
	}

	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("rag_source_id = ?", srcID).Delete(&model.ChannelRagSource{}).Error; err != nil {
			return err
		}
		if len(ids) == 0 {
			return nil
		}
		rows := make([]model.ChannelRagSource, 0, len(ids))
		for _, channelID := range ids {
			rows = append(rows, model.ChannelRagSource{
				OrgID:       org.ID,
				ChannelID:   channelID,
				RagSourceID: srcID,
			})
		}
		return tx.Create(&rows).Error
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "failed to set source channels", "error", err, "source_id", srcID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to save channels"})
		return
	}

	out := make([]string, len(ids))
	for i, id := range ids {
		out[i] = id.String()
	}
	writeJSON(w, http.StatusOK, sourceChannelsResponse{ChannelIDs: out})
}
