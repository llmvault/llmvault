package handler

import (
	"errors"
	"net/http"

	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	ragdb "github.com/usehivy/hivy/internal/rag/db"
)

// Knowledge (RAG) grants are TEAM-level now (team_rag_sources). The old
// source-side per-channel grant path (PUT /rag/sources/{id}/channels) was the
// source-side counterpart to the channel escalation hole and has been removed.
// This file keeps only the READ, derived from team grants: the channels that
// can search a source are the channels whose team was granted the source.

type sourceChannelsResponse struct {
	ChannelIDs []string `json:"channel_ids"`
}

// GetSourceChannels handles GET /v1/rag/sources/{id}/channels.
// It returns the channels that can currently search this source, derived from
// team grants: a channel can search the source iff the channel's team has been
// granted it (team_rag_sources).
// @Summary List which channels can search a source
// @Description Returns the ids of the channels that can search this knowledge source. Derived from team grants — a channel can search a source when its team has been granted the source by an org admin.
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

	// Non-manager members only learn about channels they can use; managers and
	// API-key callers see every channel whose team was granted the source.
	orgWide, userID, err := actorSeesOrgWide(r.Context(), h.db, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	teamGrant := h.db.Model(&model.TeamRagSource{}).
		Select("team_id").
		Where("rag_source_id = ? AND org_id = ?", srcID, org.ID)
	q := h.db.WithContext(r.Context()).
		Model(&model.Channel{}).
		Where("channels.org_id = ? AND channels.archived_at IS NULL", org.ID).
		Where("channels.team_id IN (?)", teamGrant)
	if !orgWide {
		q = q.Where("channels.team_id IN (?)", visibleTeamSubquery(h.db, userID))
	}
	var ids []string
	if err := q.Order("channels.name").Pluck("channels.id", &ids).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load channels"})
		return
	}
	writeJSON(w, http.StatusOK, sourceChannelsResponse{ChannelIDs: ids})
}
