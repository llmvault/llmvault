package handler

import (
	"context"
	"io"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/transcription"
)

// TranscriptionHandler transcribes audio at the organization scope, without a
// session. It powers settings surfaces (e.g. company context) where the same
// voice-to-text UX as the chat composer is wanted but there is no session to
// attach the audio to. It reuses the session transcription stack: the same
// ElevenLabs model, credential resolution, and usage accounting.
type TranscriptionHandler struct {
	db          *gorm.DB
	enqueuer    enqueue.TaskEnqueuer
	kms         *crypto.KeyWrapper
	transcriber transcription.Transcriber
	registry    *registry.Registry
}

func NewTranscriptionHandler(
	db *gorm.DB,
	enqueuer enqueue.TaskEnqueuer,
	kms *crypto.KeyWrapper,
	transcriber transcription.Transcriber,
	reg *registry.Registry,
) *TranscriptionHandler {
	return &TranscriptionHandler{
		db:          db,
		enqueuer:    enqueuer,
		kms:         kms,
		transcriber: transcriber,
		registry:    reg,
	}
}

// Create handles POST /v1/transcriptions.
// @Summary Transcribe an audio recording
// @Description Transcribes an uploaded audio file for the current organization and returns text only. Org-scoped; not tied to a session.
// @Tags transcriptions
// @Accept multipart/form-data
// @Produce json
// @Param file formData file true "Audio file to transcribe"
// @Param language_code formData string false "Optional BCP-47 language hint"
// @Success 200 {object} transcribeSessionAudioResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 422 {object} errorResponse
// @Failure 502 {object} errorResponse
// @Failure 503 {object} errorResponse
// @Security BearerAuth
// @Router /v1/transcriptions [post]
func (h *TranscriptionHandler) Create(w http.ResponseWriter, r *http.Request) {
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok || org == nil {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "no organization context"})
		return
	}
	if h.transcriber == nil || h.kms == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "audio transcription unavailable"})
		return
	}

	r.Body = http.MaxBytesReader(w, r.Body, sessionTranscriptionMaxAudioBytes+(1<<20))
	if err := r.ParseMultipartForm(sessionTranscriptionMaxAudioBytes + (1 << 20)); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid multipart form"})
		return
	}

	languageCode := strings.TrimSpace(r.FormValue("language_code"))
	if len(languageCode) > 16 {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid language_code"})
		return
	}

	file, header, err := r.FormFile("file")
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "file is required"})
		return
	}
	defer file.Close()

	if header.Size > sessionTranscriptionMaxAudioBytes {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "audio file exceeds maximum size"})
		return
	}

	contentType := strings.TrimSpace(header.Header.Get("Content-Type"))
	if contentType == "" {
		contentType = "audio/webm"
	}
	if !isAudioContentType(contentType) {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "file is not a supported audio type"})
		return
	}

	audio, err := io.ReadAll(io.LimitReader(file, sessionTranscriptionMaxAudioBytes+1))
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to read audio"})
		return
	}
	if len(audio) == 0 {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "audio file is empty"})
		return
	}
	if len(audio) > sessionTranscriptionMaxAudioBytes {
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "audio file exceeds maximum size"})
		return
	}

	reg := h.registry
	if reg == nil {
		reg = registry.Global()
	}
	route, ok := reg.ResolveModel(sessionTranscriptionProviderID, sessionTranscriptionCanonicalModel)
	if !ok {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "audio transcription model unavailable"})
		return
	}
	cred, err := credentials.ResolveForModel(r.Context(), h.db, reg, org.ID, sessionTranscriptionCanonicalModel)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "org transcription credential resolution failed", "error", err, "org_id", org.ID)
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "ElevenLabs credential unavailable"})
		return
	}
	apiKey, err := decryptCredentialKey(r.Context(), h.kms, cred)
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "org transcription credential decrypt failed", "error", err, "credential_id", cred.ID)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to decrypt transcription credential"})
		return
	}
	defer func() {
		for i := range apiKey {
			apiKey[i] = 0
		}
	}()

	filename := cleanUploadFilename(header.Filename)
	if filename == "" {
		filename = "voice.webm"
	}

	ctx, cancel := context.WithTimeout(r.Context(), sessionTranscriptionTimeout)
	defer cancel()
	result, err := h.transcriber.Transcribe(ctx, transcription.Request{
		APIKey:       apiKey,
		BaseURL:      cred.BaseURL,
		ModelID:      route.UpstreamID,
		Audio:        audio,
		Filename:     filename,
		ContentType:  contentType,
		LanguageCode: languageCode,
	})
	if err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "org transcription upstream failed", "error", err, "org_id", org.ID, "credential_id", cred.ID)
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "audio transcription failed"})
		return
	}

	h.trackOrgTranscriptionUsage(r.Context(), org.ID, cred, route.UpstreamID, currentUserID(r), languageCode, contentType, filename, len(audio), result)

	writeJSON(w, http.StatusOK, transcribeSessionAudioResponse{Text: result.Text})
}

func (h *TranscriptionHandler) trackOrgTranscriptionUsage(
	ctx context.Context,
	orgID uuid.UUID,
	cred *model.Credential,
	upstreamModel, userID, languageCode, contentType, filename string,
	bytes int,
	result transcription.Result,
) {
	if cred == nil {
		return
	}
	cost := sessionTranscriptionCostUSD(result.DurationSeconds)
	payload := buildModelUsagePayload(modelUsageInput{
		Operation:      "audio_transcription",
		OrgID:          orgID,
		ProviderID:     sessionTranscriptionProviderID,
		Model:          sessionTranscriptionCanonicalModel,
		RequestPath:    "/v1/transcriptions",
		TokenJTI:       "system:transcriptions",
		UserID:         userID,
		Credential:     cred,
		Cost:           cost,
		UpstreamStatus: http.StatusOK,
		Metadata: model.JSON{
			"filename":          filename,
			"content_type":      contentType,
			"bytes":             bytes,
			"duration_seconds":  result.DurationSeconds,
			"language_code":     strings.TrimSpace(languageCode),
			"detected_language": result.LanguageCode,
			"upstream_model":    upstreamModel,
		},
	})
	dispatchModelUsage(ctx, h.db, h.enqueuer, payload)
}
