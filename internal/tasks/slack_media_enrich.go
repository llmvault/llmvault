package tasks

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	slacksdk "github.com/slack-go/slack"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/credentials"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/system"
	"github.com/usehivy/hivy/internal/transcription"
)

const (
	slackImageDescribeModel    = "gemini-3.5-flash"
	slackImageDescribeProvider = "openrouter"
	slackAudioModel            = "scribe-v2"
	slackMediaMaxBytes         = 25 * 1024 * 1024
	slackMediaMaxItems         = 6
)

type SlackMediaEnricher interface {
	EnrichSlackMedia(ctx context.Context, token string, orgID uuid.UUID, message slacksdk.Message) string
}

type SlackMediaEnricherConfig struct {
	DB          *gorm.DB
	KMS         *crypto.KeyWrapper
	Registry    *registry.Registry
	Gateway     system.Gateway
	Transcriber transcription.Transcriber
	Enqueuer    enqueue.TaskEnqueuer
	HTTPClient  *http.Client
}

type slackMediaEnricher struct {
	db          *gorm.DB
	kms         *crypto.KeyWrapper
	registry    *registry.Registry
	gateway     system.Gateway
	transcriber transcription.Transcriber
	enqueuer    enqueue.TaskEnqueuer
	httpClient  *http.Client
}

func NewSlackMediaEnricher(cfg SlackMediaEnricherConfig) SlackMediaEnricher {
	if cfg.DB == nil {
		return nil
	}
	reg := cfg.Registry
	if reg == nil {
		reg = registry.Global()
	}
	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &slackMediaEnricher{
		db:          cfg.DB,
		kms:         cfg.KMS,
		registry:    reg,
		gateway:     cfg.Gateway,
		transcriber: cfg.Transcriber,
		enqueuer:    cfg.Enqueuer,
		httpClient:  client,
	}
}

func EnrichSlackMedia(ctx context.Context, enricher SlackMediaEnricher, token string, orgID uuid.UUID, message slacksdk.Message) string {
	if enricher == nil {
		return ""
	}
	return enricher.EnrichSlackMedia(ctx, token, orgID, message)
}

func (e *slackMediaEnricher) EnrichSlackMedia(ctx context.Context, token string, orgID uuid.UUID, message slacksdk.Message) string {
	items := slackapp.SlackMessageMediaItems(message)
	if len(items) == 0 {
		return ""
	}
	if len(items) > slackMediaMaxItems {
		items = items[:slackMediaMaxItems]
	}
	sections := make([]string, 0, len(items))
	for _, item := range items {
		switch item.Kind {
		case "image":
			sections = append(sections, e.enrichSlackImage(ctx, token, item))
		case "audio":
			sections = append(sections, e.enrichSlackAudio(ctx, token, orgID, item))
		}
	}
	return strings.TrimSpace(strings.Join(sections, "\n\n"))
}

func (e *slackMediaEnricher) downloadSlackMedia(ctx context.Context, token string, item slackapp.SlackMediaItem) ([]byte, string, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, item.URL, nil)
	if err != nil {
		return nil, "", fmt.Sprintf("download request failed: %v", err)
	}
	if strings.TrimSpace(token) != "" {
		req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	}
	resp, err := e.httpClient.Do(req)
	if err != nil {
		return nil, "", fmt.Sprintf("download failed: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, "", fmt.Sprintf("download returned status %d", resp.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, slackMediaMaxBytes+1))
	if err != nil {
		return nil, "", fmt.Sprintf("download read failed: %v", err)
	}
	if len(data) > slackMediaMaxBytes {
		return nil, "", "download exceeded size limit"
	}
	mimeType := strings.TrimSpace(item.MimeType)
	if mimeType == "" || mimeType == "application/octet-stream" {
		mimeType = resp.Header.Get("Content-Type")
	}
	if mimeType == "" {
		mimeType = http.DetectContentType(data)
	}
	return data, strings.Split(strings.TrimSpace(mimeType), ";")[0], ""
}

func decryptSlackMediaCredential(ctx context.Context, kms *crypto.KeyWrapper, cred *model.Credential) ([]byte, error) {
	dek, err := kms.Unwrap(ctx, cred.WrappedDEK)
	if err != nil {
		return nil, fmt.Errorf("kms unwrap: %w", err)
	}
	defer zeroBytes(dek)
	apiKey, err := crypto.DecryptCredential(cred.EncryptedKey, dek)
	if err != nil {
		return nil, fmt.Errorf("decrypt: %w", err)
	}
	return apiKey, nil
}

func zeroBytes(value []byte) {
	for i := range value {
		value[i] = 0
	}
}

func dataURL(mimeType string, data []byte) string {
	if strings.TrimSpace(mimeType) == "" {
		mimeType = "application/octet-stream"
	}
	return "data:" + mimeType + ";base64," + base64.StdEncoding.EncodeToString(data)
}

func parseSlackImageAnalysis(raw string) map[string]any {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	start := strings.Index(raw, "{")
	end := strings.LastIndex(raw, "}")
	if start >= 0 && end > start {
		raw = raw[start : end+1]
	}
	dec := json.NewDecoder(bytes.NewBufferString(raw))
	dec.UseNumber()
	var out map[string]any
	if err := dec.Decode(&out); err != nil {
		return map[string]any{"summary": raw}
	}
	return out
}

func slackMediaCredential(ctx context.Context, db *gorm.DB, provider string) (*model.Credential, error) {
	var cred model.Credential
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", provider).
		Order("created_at DESC").
		First(&cred).Error; err != nil {
		return nil, err
	}
	return &cred, nil
}

func resolveSlackAudioCredential(ctx context.Context, db *gorm.DB, reg *registry.Registry, orgID uuid.UUID) (*model.Credential, error) {
	return credentials.ResolveForModel(ctx, db, reg, orgID, slackAudioModel)
}
