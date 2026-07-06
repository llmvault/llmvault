package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

// ObservationHandler serves the human feedback loop over consolidated memory
// observations: confirm, correct, delete (with resurrection suppression), and
// pin-to-directive.
type ObservationHandler struct {
	db *gorm.DB
}

func NewObservationHandler(db *gorm.DB) *ObservationHandler {
	return &ObservationHandler{db: db}
}

type observationResponse struct {
	ID              string     `json:"id"`
	ChannelID       *string    `json:"channel_id,omitempty"`
	Content         string     `json:"content"`
	Kind            string     `json:"kind"`
	Entities        []string   `json:"entities"`
	ProofCount      int        `json:"proof_count"`
	LastMentionedAt string     `json:"last_mentioned_at"`
	ExpiresAt       *string    `json:"expires_at,omitempty"`
	HumanVerified   bool       `json:"human_verified"`
	Metadata        model.JSON `json:"metadata"`
	CreatedAt       string     `json:"created_at"`
	ArchivedAt      *string    `json:"archived_at,omitempty"`
}

type observationListResponse struct {
	Data    []observationResponse `json:"data"`
	HasMore bool                  `json:"has_more"`
}

type observationCorrectRequest struct {
	Content string `json:"content"`
}

// List handles GET /v1/observations.
// @Summary List memory observations
// @Description Lists consolidated memory observations, non-archived first. channel_id filters: a channel UUID for that channel, "org" for org-wide, omitted for all. Paginate with limit/offset.
// @Tags observations
// @Produce json
// @Param channel_id query string false "Filter: channel UUID, \"org\", or omitted for all"
// @Param limit query int false "Max items (1-100, default 50)"
// @Param offset query int false "Pagination offset"
// @Success 200 {object} observationListResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Security BearerAuth
// @Router /v1/observations [get]
func (h *ObservationHandler) List(w http.ResponseWriter, r *http.Request) {
	org, ok := orgForChannelRequest(w, r.Context())
	if !ok {
		return
	}
	query := h.db.WithContext(r.Context()).Where("org_id = ?", org.ID)
	switch raw := strings.TrimSpace(r.URL.Query().Get("channel_id")); raw {
	case "":
	case "org":
		query = query.Where("channel_id IS NULL")
	default:
		channelID, err := uuid.Parse(raw)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid channel_id"})
			return
		}
		query = query.Where("channel_id = ?", channelID)
	}
	limit := parseMemoryLimit(r.URL.Query().Get("limit"), 50)
	offset, _ := strconv.Atoi(strings.TrimSpace(r.URL.Query().Get("offset")))
	if offset < 0 {
		offset = 0
	}
	var rows []model.AgentObservation
	if err := query.
		Order("(archived_at IS NULL) DESC, last_mentioned_at DESC, created_at DESC").
		Limit(limit + 1).
		Offset(offset).
		Find(&rows).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "list observations", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to list observations"})
		return
	}
	hasMore := len(rows) > limit
	if hasMore {
		rows = rows[:limit]
	}
	out := make([]observationResponse, len(rows))
	for i := range rows {
		out[i] = observationToResponse(rows[i])
	}
	writeJSON(w, http.StatusOK, observationListResponse{Data: out, HasMore: hasMore})
}

// Confirm handles POST /v1/observations/{id}/confirm.
// @Summary Confirm an observation
// @Description Human confirmation: increments proof_count and refreshes last_mentioned_at. Requires an org admin/owner.
// @Tags observations
// @Produce json
// @Param id path string true "Observation UUID"
// @Success 200 {object} map[string]observationResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/observations/{id}/confirm [post]
func (h *ObservationHandler) Confirm(w http.ResponseWriter, r *http.Request) {
	obs, _, ok := h.authorizeObservationMutation(w, r)
	if !ok {
		return
	}
	now := time.Now()
	if err := h.db.WithContext(r.Context()).
		Model(&model.AgentObservation{}).
		Where("id = ? AND org_id = ?", obs.ID, obs.OrgID).
		Updates(map[string]any{
			"proof_count":       gorm.Expr("proof_count + 1"),
			"last_mentioned_at": now,
			"updated_at":        now,
		}).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "confirm observation", "error", err, "observation_id", obs.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to confirm observation"})
		return
	}
	obs.ProofCount++
	obs.LastMentionedAt = now
	obs.UpdatedAt = now
	h.triggerChannelDigestRecompute(r.Context(), obs.OrgID, obs.ChannelID)
	writeJSON(w, http.StatusOK, map[string]observationResponse{"observation": observationToResponse(obs)})
}

// Correct handles POST /v1/observations/{id}/correct.
// @Summary Correct an observation
// @Description Human edit: creates a new human-verified observation with the corrected content (proof count carried over) and archives the old one with a supersession link. Requires an org admin/owner.
// @Tags observations
// @Accept json
// @Produce json
// @Param id path string true "Observation UUID"
// @Param request body observationCorrectRequest true "Corrected content"
// @Success 200 {object} map[string]observationResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/observations/{id}/correct [post]
func (h *ObservationHandler) Correct(w http.ResponseWriter, r *http.Request) {
	obs, userID, ok := h.authorizeObservationMutation(w, r)
	if !ok {
		return
	}
	var req observationCorrectRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	content := strings.TrimSpace(req.Content)
	if content == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "content is required"})
		return
	}

	now := time.Now()
	// The corrected text lives on a NEW human-verified row; consolidation may
	// append evidence to it but never rewrites the text. The old row stays as
	// archived history behind the supersession link.
	replacement := model.AgentObservation{
		OrgID:           obs.OrgID,
		ChannelID:       obs.ChannelID,
		Content:         content,
		Kind:            obs.Kind,
		Entities:        obs.Entities,
		ProofCount:      obs.ProofCount,
		SourceFactIDs:   obs.SourceFactIDs,
		OccurredStart:   obs.OccurredStart,
		OccurredEnd:     obs.OccurredEnd,
		LastMentionedAt: now,
		ExpiresAt:       obs.ExpiresAt,
		HumanVerified:   true,
		Metadata: appendObservationAudit(obs.Metadata, model.JSON{
			"op":               "correct",
			"at":               now.UTC().Format(time.RFC3339),
			"by_user_id":       formatUUIDPtr(userID),
			"supersedes":       obs.ID.String(),
			"previous_content": obs.Content,
			"reason":           "human correction",
		}),
	}
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Org", "Channel").Create(&replacement).Error; err != nil {
			return err
		}
		return tx.Model(&model.AgentObservation{}).
			Where("id = ? AND org_id = ?", obs.ID, obs.OrgID).
			Updates(map[string]any{
				"archived_at":   now,
				"superseded_by": replacement.ID,
				"updated_at":    now,
			}).Error
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "correct observation", "error", err, "observation_id", obs.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to correct observation"})
		return
	}
	h.triggerChannelDigestRecompute(r.Context(), obs.OrgID, obs.ChannelID)
	writeJSON(w, http.StatusOK, map[string]observationResponse{"observation": observationToResponse(replacement)})
}

// Delete handles DELETE /v1/observations/{id}.
// @Summary Delete an observation
// @Description Archives the observation and records its content fingerprint in the per-channel suppression list so consolidation cannot resurrect it. Requires an org admin/owner.
// @Tags observations
// @Produce json
// @Param id path string true "Observation UUID"
// @Success 200 {object} map[string]string
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/observations/{id} [delete]
func (h *ObservationHandler) Delete(w http.ResponseWriter, r *http.Request) {
	obs, _, ok := h.authorizeObservationMutation(w, r)
	if !ok {
		return
	}
	now := time.Now()
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&model.AgentObservation{}).
			Where("id = ? AND org_id = ?", obs.ID, obs.OrgID).
			Updates(map[string]any{"archived_at": now, "updated_at": now}).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{DoNothing: true}).
			Omit("Org", "Channel").
			Create(&model.MemorySuppression{
				OrgID:              obs.OrgID,
				ChannelID:          obs.ChannelID,
				ContentFingerprint: memory.SuppressionFingerprint(obs.Content),
			}).Error
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "delete observation", "error", err, "observation_id", obs.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to delete observation"})
		return
	}
	h.triggerChannelDigestRecompute(r.Context(), obs.OrgID, obs.ChannelID)
	writeJSON(w, http.StatusOK, map[string]string{"status": "archived"})
}

// Pin handles POST /v1/observations/{id}/pin.
// @Summary Pin an observation to a directive
// @Description Promotes the observation into an agent directive (source extracted-confirmed) in the observation's scope, and marks the observation as pinned. Requires an org admin/owner.
// @Tags observations
// @Produce json
// @Param id path string true "Observation UUID"
// @Success 201 {object} map[string]any
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Security BearerAuth
// @Router /v1/observations/{id}/pin [post]
func (h *ObservationHandler) Pin(w http.ResponseWriter, r *http.Request) {
	obs, userID, ok := h.authorizeObservationMutation(w, r)
	if !ok {
		return
	}
	now := time.Now()
	directive := model.AgentDirective{
		OrgID:           obs.OrgID,
		ChannelID:       obs.ChannelID,
		Content:         obs.Content,
		CreatedByUserID: userID,
		Source:          model.DirectiveSourceExtractedConfirmed,
		Active:          true,
	}
	metadata := appendObservationAudit(obs.Metadata, model.JSON{
		"op":         "pin",
		"at":         now.UTC().Format(time.RFC3339),
		"by_user_id": formatUUIDPtr(userID),
		"reason":     "pinned to directive",
	})
	metadata["pinned"] = true
	err := h.db.WithContext(r.Context()).Transaction(func(tx *gorm.DB) error {
		if err := tx.Omit("Org", "Channel", "CreatedBy").Create(&directive).Error; err != nil {
			return err
		}
		return tx.Model(&model.AgentObservation{}).
			Where("id = ? AND org_id = ?", obs.ID, obs.OrgID).
			Updates(map[string]any{"metadata": metadata, "updated_at": now}).Error
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "pin observation", "error", err, "observation_id", obs.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to pin observation"})
		return
	}
	obs.Metadata = metadata
	obs.UpdatedAt = now
	writeJSON(w, http.StatusCreated, map[string]any{
		"directive":   directiveToResponse(directive),
		"observation": observationToResponse(obs),
	})
}

func (h *ObservationHandler) authorizeObservationMutation(w http.ResponseWriter, r *http.Request) (model.AgentObservation, *uuid.UUID, bool) {
	var obs model.AgentObservation
	org, ok := orgForChannelRequest(w, r.Context())
	if !ok {
		return obs, nil, false
	}
	id, err := uuid.Parse(chi.URLParam(r, "id"))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid observation id"})
		return obs, nil, false
	}
	userID, _ := currentRequestUserID(r.Context())
	if !canManageMemoryResources(r, h.db, org.ID, userID) {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "admin role required to manage observations"})
		return obs, nil, false
	}
	err = h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ? AND archived_at IS NULL", id, org.ID).
		First(&obs).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "observation not found"})
		return obs, nil, false
	}
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load observation"})
		return obs, nil, false
	}
	return obs, userID, true
}

// triggerChannelDigestRecompute refreshes the precomputed channel memory
// digest after a human feedback mutation so recall reflects it immediately.
// Best-effort in the background: a failed recompute is logged, never surfaced,
// and the periodic consolidation sweep repairs it. For org-wide observations
// (nil channelID) every channel folding org memories is refreshed.
func (h *ObservationHandler) triggerChannelDigestRecompute(ctx context.Context, orgID uuid.UUID, channelID *uuid.UUID) {
	service := memory.NewService(memory.Config{DB: h.db})
	bg := context.WithoutCancel(ctx)
	goroutine.Go(bg, func(bg context.Context) {
		if channelID != nil {
			if err := service.RecomputeChannelDigest(bg, orgID, *channelID); err != nil {
				logging.FromContext(bg).WarnContext(bg, "recompute channel memory digest", "error", err, "channel_id", *channelID)
			}
			return
		}
		var channelIDs []uuid.UUID
		if err := h.db.WithContext(bg).Model(&model.Channel{}).
			Where("org_id = ? AND archived_at IS NULL AND expose_org_memories = true", orgID).
			Pluck("id", &channelIDs).Error; err != nil {
			logging.FromContext(bg).WarnContext(bg, "list channels for digest recompute", "error", err, "org_id", orgID)
			return
		}
		for _, id := range channelIDs {
			if err := service.RecomputeChannelDigest(bg, orgID, id); err != nil {
				logging.FromContext(bg).WarnContext(bg, "recompute channel memory digest", "error", err, "channel_id", id)
			}
		}
	})
}

// appendObservationAudit returns a copy of metadata with entry appended to its
// "audit" array.
func appendObservationAudit(metadata model.JSON, entry model.JSON) model.JSON {
	out := model.JSON{}
	for key, value := range metadata {
		out[key] = value
	}
	audit, _ := out["audit"].([]any)
	out["audit"] = append(audit, map[string]any(entry))
	return out
}

func observationToResponse(obs model.AgentObservation) observationResponse {
	metadata := obs.Metadata
	if metadata == nil {
		metadata = model.JSON{}
	}
	return observationResponse{
		ID:              obs.ID.String(),
		ChannelID:       formatUUIDPtr(obs.ChannelID),
		Content:         obs.Content,
		Kind:            obs.Kind,
		Entities:        []string(obs.Entities),
		ProofCount:      obs.ProofCount,
		LastMentionedAt: obs.LastMentionedAt.UTC().Format(time.RFC3339),
		ExpiresAt:       formatTimePtr(obs.ExpiresAt),
		HumanVerified:   obs.HumanVerified,
		Metadata:        metadata,
		CreatedAt:       obs.CreatedAt.UTC().Format(time.RFC3339),
		ArchivedAt:      formatTimePtr(obs.ArchivedAt),
	}
}
