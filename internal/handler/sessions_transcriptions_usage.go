package handler

import (
	"context"
	"net/http"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/transcription"
)

func (h *SessionHandler) trackSessionTranscriptionUsage(ctx context.Context, session model.Session, asset model.AgentAsset, cred *model.Credential, upstreamModel, userID, languageCode string, result transcription.Result) {
	if cred == nil {
		return
	}
	cost := sessionTranscriptionCostUSD(result.DurationSeconds)
	payload := buildModelUsagePayload(modelUsageInput{
		Operation:      "audio_transcription",
		OrgID:          session.OrgID,
		ProviderID:     sessionTranscriptionProviderID,
		Model:          sessionTranscriptionCanonicalModel,
		RequestPath:    "/v1/sessions/" + session.ID.String() + "/transcriptions",
		TokenJTI:       "system:sessions.transcriptions",
		UserID:         userID,
		Session:        &session,
		SandboxID:      &asset.SandboxID,
		Credential:     cred,
		Cost:           cost,
		UpstreamStatus: http.StatusOK,
		Metadata: model.JSON{
			"drive_asset_id":    asset.ID.String(),
			"filename":          asset.Filename,
			"content_type":      asset.ContentType,
			"bytes":             asset.Bytes,
			"duration_seconds":  result.DurationSeconds,
			"language_code":     strings.TrimSpace(languageCode),
			"detected_language": result.LanguageCode,
			"upstream_model":    upstreamModel,
		},
	})
	dispatchModelUsage(ctx, h.db, h.enqueuer, payload)
}

func sessionTranscriptionCostUSD(durationSeconds float64) float64 {
	if durationSeconds <= 0 {
		return 0
	}
	return durationSeconds / 3600 * sessionTranscriptionCostPerHourUSD
}
