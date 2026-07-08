package tasks

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/oklog/ulid/v2"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/transcription"
)

const slackAudioTranscriptionTimeout = 60 * time.Second

func (e *slackMediaEnricher) enrichSlackAudio(ctx context.Context, token string, orgID uuid.UUID, item slackapp.SlackMediaItem) string {
	data, mimeType, notice := e.downloadSlackMedia(ctx, token, item)
	if notice != "" {
		return slackMediaNoticeXML("audio", item, item.MimeType, notice)
	}
	if e.transcriber == nil || e.kms == nil {
		return slackMediaNoticeXML("audio", item, mimeType, "audio transcription service unavailable")
	}
	route, ok := e.registry.ResolveModel("elevenlabs", slackAudioModel)
	if !ok {
		return slackMediaNoticeXML("audio", item, mimeType, "audio transcription model unavailable")
	}
	cred, err := resolveSlackAudioCredential(ctx, e.db, e.registry, orgID)
	if err != nil {
		return slackMediaNoticeXML("audio", item, mimeType, "audio transcription credential unavailable")
	}
	apiKey, err := decryptSlackMediaCredential(ctx, e.kms, cred)
	if err != nil {
		return slackMediaNoticeXML("audio", item, mimeType, "audio transcription credential decrypt failed")
	}
	defer zeroBytes(apiKey)

	transcribeCtx, cancel := context.WithTimeout(ctx, slackAudioTranscriptionTimeout)
	defer cancel()
	result, err := e.transcriber.Transcribe(transcribeCtx, transcription.Request{
		APIKey:       apiKey,
		BaseURL:      cred.BaseURL,
		ModelID:      route.UpstreamID,
		Audio:        data,
		Filename:     item.Name,
		ContentType:  mimeType,
		LanguageCode: "en",
	})
	if err != nil {
		return slackMediaNoticeXML("audio", item, mimeType, "audio transcription failed: "+err.Error())
	}
	e.trackSlackAudioUsage(ctx, orgID, cred, result)
	return slackAudioXML(item, mimeType, result)
}

func (e *slackMediaEnricher) trackSlackAudioUsage(ctx context.Context, orgID uuid.UUID, cred *model.Credential, result transcription.Result) {
	if cred == nil {
		return
	}
	payload := ModelUsageWritePayload{
		Generation: model.Generation{
			ID:             "gen_" + ulid.Make().String(),
			OrgID:          orgID,
			CredentialID:   cred.ID,
			TokenJTI:       "system:slack.media.audio",
			ProviderID:     "elevenlabs",
			Model:          slackAudioModel,
			RequestPath:    "slack:media/audio",
			Cost:           transcription.CostUSD(result.DurationSeconds),
			UpstreamStatus: http.StatusOK,
			Tags:           pq.StringArray{"model_usage", "audio_transcription"},
			CreatedAt:      time.Now().UTC(),
			IsSystem:       cred.OrgID == nil,
		},
	}
	if e.enqueuer != nil {
		if err := EnqueueModelUsageWrite(ctx, e.enqueuer, payload); err != nil {
			logging.Capture(ctx, fmt.Errorf("enqueue slack audio usage: %w", err))
		}
		return
	}
	if e.db != nil {
		if err := WriteModelUsage(ctx, e.db, payload); err != nil {
			logging.Capture(ctx, fmt.Errorf("write slack audio usage: %w", err))
		}
	}
}

func slackAudioXML(item slackapp.SlackMediaItem, mimeType string, result transcription.Result) string {
	var sections []string
	if text := strings.TrimSpace(result.Text); text != "" {
		sections = append(sections, "<transcript>\n"+escapeXMLText(text)+"\n</transcript>")
	}
	if result.LanguageCode != "" {
		sections = append(sections, "<language_code>"+escapeXMLText(result.LanguageCode)+"</language_code>")
	}
	if result.DurationSeconds > 0 {
		sections = append(sections, fmt.Sprintf("<duration_seconds>%.2f</duration_seconds>", result.DurationSeconds))
	}
	if len(sections) == 0 {
		sections = append(sections, "<notice>Audio was downloaded but no transcript was produced.</notice>")
	}
	return slackAttachmentXML("audio", item, mimeType, strings.Join(sections, "\n"))
}
