package tasks

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

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
	return slackAudioXML(item, mimeType, result)
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
