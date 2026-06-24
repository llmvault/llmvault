package handler

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/transcription"
)

const (
	sessionTranscriptionCanonicalModel = "scribe-v2"
	sessionTranscriptionProviderID     = "elevenlabs"
	sessionTranscriptionTimeout        = 60 * time.Second
	sessionTranscriptionMaxAudioBytes  = 25 * 1024 * 1024
)

type transcribeSessionAudioRequest struct {
	DriveAssetID string `json:"drive_asset_id"`
	LanguageCode string `json:"language_code,omitempty"`
}

type transcribeSessionAudioResponse struct {
	Text string `json:"text"`
}

// TranscribeAudio handles POST /v1/sessions/{id}/transcriptions.
// @Summary Transcribe a session audio asset
// @Description Transcribes an uploaded drive audio asset for a session and returns text only.
// @Tags sessions
// @Accept json
// @Produce json
// @Param id path string true "Session ID"
// @Param body body transcribeSessionAudioRequest true "Transcription payload"
// @Success 200 {object} transcribeSessionAudioResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/transcriptions [post]
func (h *SessionHandler) TranscribeAudio(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authorizeSession(w, r, true)
	if !ok {
		return
	}
	if session.Status != "active" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "session is not active"})
		return
	}
	if h.transcriptionReader == nil || h.transcriptionTranscriber == nil || h.transcriptionKMS == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "audio transcription unavailable"})
		return
	}

	var req transcribeSessionAudioRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	assetID, err := uuid.Parse(strings.TrimSpace(req.DriveAssetID))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid drive_asset_id"})
		return
	}
	languageCode := strings.TrimSpace(req.LanguageCode)
	if len(languageCode) > 16 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid language_code"})
		return
	}

	asset, ok := h.loadSessionAudioAsset(w, r, session, assetID)
	if !ok {
		return
	}
	audio, ok := h.readSessionAudioAsset(w, r, asset)
	if !ok {
		return
	}

	reg := h.transcriptionRegistry
	if reg == nil {
		reg = registry.Global()
	}
	route, ok := reg.ResolveModel(sessionTranscriptionProviderID, sessionTranscriptionCanonicalModel)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "audio transcription model unavailable"})
		return
	}
	cred, err := credentials.ResolveForModel(r.Context(), h.db, reg, session.OrgID, sessionTranscriptionCanonicalModel)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "session transcription credential resolution failed", "error", err, "org_id", session.OrgID)
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "ElevenLabs credential unavailable"})
		return
	}
	apiKey, err := decryptCredentialKey(r.Context(), h.transcriptionKMS, cred)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "session transcription credential decrypt failed", "error", err, "credential_id", cred.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to decrypt transcription credential"})
		return
	}
	defer func() {
		for i := range apiKey {
			apiKey[i] = 0
		}
	}()

	ctx, cancel := context.WithTimeout(r.Context(), sessionTranscriptionTimeout)
	defer cancel()
	result, err := h.transcriptionTranscriber.Transcribe(ctx, transcription.Request{
		APIKey:       apiKey,
		BaseURL:      cred.BaseURL,
		ModelID:      route.UpstreamID,
		Audio:        audio,
		Filename:     asset.Filename,
		ContentType:  asset.ContentType,
		LanguageCode: languageCode,
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "session transcription upstream failed", "error", err, "asset_id", asset.ID, "credential_id", cred.ID)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "audio transcription failed"})
		return
	}
	writeJSON(w, http.StatusOK, transcribeSessionAudioResponse{Text: result.Text})
}

func (h *SessionHandler) loadSessionAudioAsset(w http.ResponseWriter, r *http.Request, session model.Session, assetID uuid.UUID) (model.AgentAsset, bool) {
	var asset model.AgentAsset
	if err := h.db.WithContext(r.Context()).
		Where("id = ? AND org_id = ?", assetID, session.OrgID).
		First(&asset).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			writeJSON(w, http.StatusNotFound, errorResponse{Error: "asset not found"})
			return model.AgentAsset{}, false
		}
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load asset"})
		return model.AgentAsset{}, false
	}
	if asset.AgentID != session.AgentID {
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "asset not found"})
		return model.AgentAsset{}, false
	}
	if !isAudioContentType(asset.ContentType) {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "asset is not an audio file"})
		return model.AgentAsset{}, false
	}
	if asset.Bytes <= 0 || asset.Bytes > sessionTranscriptionMaxAudioBytes {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "audio file exceeds maximum size"})
		return model.AgentAsset{}, false
	}
	if strings.TrimSpace(asset.Key) == "" {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "asset file is unavailable"})
		return model.AgentAsset{}, false
	}
	return asset, true
}

func (h *SessionHandler) readSessionAudioAsset(w http.ResponseWriter, r *http.Request, asset model.AgentAsset) ([]byte, bool) {
	body, err := h.transcriptionReader.Open(r.Context(), asset.Key)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "session transcription asset read failed", "error", err, "asset_id", asset.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to read asset"})
		return nil, false
	}
	defer body.Close()

	audio, err := io.ReadAll(io.LimitReader(body, sessionTranscriptionMaxAudioBytes+1))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to read asset"})
		return nil, false
	}
	if len(audio) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "audio file is empty"})
		return nil, false
	}
	if len(audio) > sessionTranscriptionMaxAudioBytes {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "audio file exceeds maximum size"})
		return nil, false
	}
	return audio, true
}

func isAudioContentType(contentType string) bool {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	switch contentType {
	case "audio/m4a",
		"audio/mp3",
		"audio/mp4",
		"audio/mpeg",
		"audio/ogg",
		"audio/wav",
		"audio/webm",
		"audio/x-m4a",
		"audio/x-wav",
		"video/mp4",
		"video/webm":
		return true
	default:
		return false
	}
}
