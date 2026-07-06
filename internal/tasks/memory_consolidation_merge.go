package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	openai "github.com/sashabaranov/go-openai"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/providerheaders"
)

func (h *MemoryConsolidationHandler) mergeObservations(
	ctx context.Context,
	svc *memory.Service,
	a, b *model.AgentObservation,
	decision consolidationDedupDecision,
	now time.Time,
) error {
	keep, drop := a, b
	if b.CreatedAt.Before(a.CreatedAt) ||
		(b.CreatedAt.Equal(a.CreatedAt) && b.ID.String() < a.ID.String()) {
		keep, drop = b, a
	}
	mergedText := strings.TrimSpace(decision.Text)
	contentChanged := false
	if mergedText != "" && mergedText != keep.Content && !keep.HumanVerified {
		keep.Content = mergedText
		contentChanged = true
	}
	keep.ProofCount += drop.ProofCount
	keep.SourceFactIDs = pq.StringArray(unionStrings(keep.SourceFactIDs, drop.SourceFactIDs))
	keep.Entities = pq.StringArray(unionStrings(keep.Entities, drop.Entities))
	if drop.LastMentionedAt.After(keep.LastMentionedAt) {
		keep.LastMentionedAt = drop.LastMentionedAt
	}
	keep.Metadata = appendObservationAudit(keep.Metadata, "merge", decision.Reason, nil, now)
	if err := svc.SaveObservationChanges(ctx, keep, contentChanged); err != nil {
		return err
	}
	if contentChanged {
		if _, err := h.embedObservation(ctx, svc, keep); err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "re-embed after merge failed; queued for retry",
				"observation_id", keep.ID, "error", err)
		}
	}
	keepID := keep.ID
	if err := svc.ArchiveObservation(ctx, drop.OrgID, drop.ID, &keepID); err != nil &&
		!errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

func (h *MemoryConsolidationHandler) completion() consolidationCompletionFunc {
	if h.complete != nil {
		return h.complete
	}
	return h.defaultComplete
}

func (h *MemoryConsolidationHandler) defaultComplete(
	ctx context.Context,
	systemPrompt, userPrompt, schemaName string,
	schema json.RawMessage,
	maxTokens int,
) (string, error) {
	cred, apiKey, err := loadConsolidationCredential(ctx, h.db, h.cacheManager, h.modelCfg.ProviderID)
	if err != nil {
		return "", err
	}
	cfg := openai.DefaultConfig(apiKey)
	if cred.BaseURL != "" {
		cfg.BaseURL = cred.BaseURL
	}
	if providerheaders.IsOpenRouter(cred.ProviderID, cfg.BaseURL) {
		cfg.HTTPClient = consolidationHeaderDoer{inner: cfg.HTTPClient}
	}
	client := openai.NewClientWithConfig(cfg)

	// go-openai omits a zero temperature; the smallest positive float is the
	// documented way to transmit an effective 0.0.
	temperature := float32(h.modelCfg.Temperature)
	if temperature <= 0 {
		temperature = math.SmallestNonzeroFloat32
	}
	messages := make([]openai.ChatCompletionMessage, 0, 2)
	if strings.TrimSpace(systemPrompt) != "" {
		messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleSystem, Content: systemPrompt})
	}
	messages = append(messages, openai.ChatCompletionMessage{Role: openai.ChatMessageRoleUser, Content: userPrompt})
	resp, err := client.CreateChatCompletion(ctx, openai.ChatCompletionRequest{
		Model:       h.modelCfg.ModelID,
		Messages:    messages,
		MaxTokens:   maxTokens,
		Temperature: temperature,
		ResponseFormat: &openai.ChatCompletionResponseFormat{
			Type: openai.ChatCompletionResponseFormatTypeJSONSchema,
			JSONSchema: &openai.ChatCompletionResponseFormatJSONSchema{
				Name:   schemaName,
				Schema: schema,
				Strict: true,
			},
		},
	})
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", fmt.Errorf("consolidation model returned no choices")
	}
	return resp.Choices[0].Message.Content, nil
}

func loadConsolidationCredential(
	ctx context.Context,
	db *gorm.DB,
	cacheManager *cache.Manager,
	providerID string,
) (*model.Credential, string, error) {
	if db == nil || cacheManager == nil {
		return nil, "", fmt.Errorf("missing consolidation dependencies")
	}
	var cred model.Credential
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", providerID).
		Order("created_at ASC").
		First(&cred).Error; err != nil {
		return nil, "", fmt.Errorf("load system %s credential for consolidation: %w", providerID, err)
	}
	decrypted, err := cacheManager.GetDecryptedCredentialByID(ctx, cred.ID.String())
	if err != nil {
		return nil, "", fmt.Errorf("decrypt consolidation credential: %w", err)
	}
	return &cred, string(decrypted.APIKey), nil
}

type consolidationHeaderDoer struct {
	inner openai.HTTPDoer
}

func (d consolidationHeaderDoer) Do(req *http.Request) (*http.Response, error) {
	providerheaders.ApplyOpenRouter(req)
	return d.inner.Do(req)
}

// consolidationCreateChannelID applies the org-promotion gate: scope "org"
// stores channel_id NULL only when proof_count >= 2 after merge OR a source
// fact came from an explicit human actor statement; otherwise the observation
// stays channel-scoped.
func consolidationCreateChannelID(scope string, channelID uuid.UUID, proofCount int, humanSource bool) *uuid.UUID {
	if scope == "org" && (proofCount >= 2 || humanSource) {
		return nil
	}
	id := channelID
	return &id
}

// factFromHumanActor reports whether a reflection fact was attributed to a
// human actor statement (reflection sets these fields only for human
// evidence).
func factFromHumanActor(fact model.AgentMemory) bool {
	name, _ := fact.Metadata["actor_display_name"].(string)
	ref, _ := fact.Metadata["actor_external_ref"].(string)
	return strings.TrimSpace(name) != "" || strings.TrimSpace(ref) != ""
}

// applyConsolidationUpdate mutates obs per the update-op rules: bump
// proof_count, append source facts, refresh last_mentioned_at, append the
// audit entry — and NEVER rewrite content when human_verified (evidence only).
// Returns whether the content text changed (caller re-embeds when true).
func applyConsolidationUpdate(
	obs *model.AgentObservation,
	text, reason string,
	newFactIDs []uuid.UUID,
	now time.Time,
) bool {
	existing := map[string]bool{}
	for _, id := range obs.SourceFactIDs {
		existing[id] = true
	}
	added := 0
	for _, id := range newFactIDs {
		key := id.String()
		if existing[key] {
			continue
		}
		existing[key] = true
		obs.SourceFactIDs = append(obs.SourceFactIDs, key)
		added++
	}
	if added == 0 {
		added = 1 // re-mention still counts as evidence
	}
	obs.ProofCount += added
	obs.LastMentionedAt = now
	contentChanged := false
	text = strings.TrimSpace(text)
	if !obs.HumanVerified && text != "" && text != obs.Content {
		obs.Content = text
		contentChanged = true
	}
	obs.Metadata = appendObservationAudit(obs.Metadata, "update", reason, newFactIDs, now)
	return contentChanged
}

// appendObservationAudit appends one {op, reason, fact_ids, at} entry to
// metadata.audit — every applied consolidation op is audited.
func appendObservationAudit(meta model.JSON, op, reason string, factIDs []uuid.UUID, at time.Time) model.JSON {
	if meta == nil {
		meta = model.JSON{}
	}
	entry := map[string]any{
		"op":       op,
		"reason":   reason,
		"fact_ids": sortedUUIDStrings(factIDs),
		"at":       at.UTC().Format(time.RFC3339),
	}
	audit, _ := meta["audit"].([]any)
	meta["audit"] = append(audit, entry)
	return meta
}

func unionStrings(a, b []string) []string {
	seen := make(map[string]bool, len(a)+len(b))
	out := make([]string, 0, len(a)+len(b))
	for _, values := range [][]string{a, b} {
		for _, value := range values {
			if value == "" || seen[value] {
				continue
			}
			seen[value] = true
			out = append(out, value)
		}
	}
	return out
}

// MemoryConsolidationSweepHandler is the periodic stranded-facts sweep: any
// channel holding reflection facts with consolidated_at IS NULL gets a
// consolidation run enqueued.
