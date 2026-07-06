package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"github.com/lib/pq"
	openai "github.com/sashabaranov/go-openai"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/providerheaders"
)

const (
	memoryConsolidationBatchSize      = 30
	memoryConsolidationTopK           = 5
	memoryConsolidationDedupThreshold = 0.9
	memoryConsolidationDedupMaxTokens = 800
	memoryConsolidationSweepLimit     = 200

	consolidationDefaultProviderID = "openrouter"
	// gpt-5-mini through the OpenRouter upstream id (the registry catalog has
	// no gpt-5-mini canonical entry, so the upstream id is used directly).
	consolidationDefaultModelID = "openai/gpt-5-mini"
)

// consolidationModelConfig holds the dedicated consolidation model knobs —
// separate from session naming and reflection per the memory plan.
type consolidationModelConfig struct {
	ProviderID  string
	ModelID     string
	Temperature float64
}

func consolidationModelConfigFromEnv() consolidationModelConfig {
	cfg := consolidationModelConfig{
		ProviderID:  consolidationDefaultProviderID,
		ModelID:     consolidationDefaultModelID,
		Temperature: 0,
	}
	if v := strings.TrimSpace(os.Getenv("HIVY_CONSOLIDATION_PROVIDER")); v != "" {
		cfg.ProviderID = v
	}
	if v := strings.TrimSpace(os.Getenv("HIVY_CONSOLIDATION_MODEL")); v != "" {
		cfg.ModelID = v
	}
	if v := strings.TrimSpace(os.Getenv("HIVY_CONSOLIDATION_TEMPERATURE")); v != "" {
		if parsed, err := strconv.ParseFloat(v, 64); err == nil && parsed >= 0 {
			cfg.Temperature = parsed
		}
	}
	return cfg
}

// consolidationCompletionFunc is the LLM seam: production uses the default
// OpenAI-compatible call (temperature-honoring); tests inject a stub.
type consolidationCompletionFunc func(
	ctx context.Context,
	systemPrompt, userPrompt, schemaName string,
	schema json.RawMessage,
	maxTokens int,
) (string, error)

// MemoryConsolidationHandler folds a channel's unconsolidated reflection
// facts into canonical observations: one LLM call proposing
// creates/updates/deletes, applied with the promotion/suppression/verified
// guards, followed by a semantic dedup post-pass and a digest recompute.
type MemoryConsolidationHandler struct {
	db           *gorm.DB
	cacheManager *cache.Manager
	enqueuer     enqueue.TaskEnqueuer
	memoryCfg    MemoryEmbeddingConfig
	modelCfg     consolidationModelConfig
	complete     consolidationCompletionFunc
	now          func() time.Time
}

func NewMemoryConsolidationHandler(
	db *gorm.DB,
	cacheManager *cache.Manager,
	enqueuer enqueue.TaskEnqueuer,
	memoryCfg MemoryEmbeddingConfig,
) *MemoryConsolidationHandler {
	return &MemoryConsolidationHandler{
		db:           db,
		cacheManager: cacheManager,
		enqueuer:     enqueuer,
		memoryCfg:    memoryCfg,
		modelCfg:     consolidationModelConfigFromEnv(),
	}
}

func (h *MemoryConsolidationHandler) memoryService() *memory.Service {
	return memory.NewService(memory.Config{
		DB:             h.db,
		CacheManager:   h.cacheManager,
		EmbeddingModel: h.memoryCfg.Model,
		EmbeddingDim:   h.memoryCfg.Dim,
	})
}

func (h *MemoryConsolidationHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload MemoryConsolidatePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal memory consolidate payload: %w", err)
	}
	now := time.Now().UTC()
	if h.now != nil {
		now = h.now().UTC()
	}
	logger := logging.FromContext(ctx)
	svc := h.memoryService()

	facts, err := svc.ListUnconsolidatedFacts(ctx, payload.OrgID, payload.ChannelID, memoryConsolidationBatchSize)
	if err != nil {
		return err
	}
	if len(facts) == 0 {
		return nil
	}

	// 2. Per fact: vector top-K similar non-archived channel + org-wide
	// observations; pool and dedupe.
	contents := make([]string, len(facts))
	for i, fact := range facts {
		contents[i] = fact.Content
	}
	vectors, err := svc.EmbedContents(ctx, contents)
	if err != nil {
		return fmt.Errorf("embed facts for consolidation: %w", err)
	}
	channelID := payload.ChannelID
	pool := map[uuid.UUID]model.AgentObservation{}
	for i := range facts {
		hits, err := svc.SimilarObservations(ctx, memory.SimilarObservationsRequest{
			OrgID:          payload.OrgID,
			ChannelID:      &channelID,
			IncludeOrgWide: true,
			Vector:         vectors[i],
			Limit:          memoryConsolidationTopK,
		})
		if err != nil {
			return err
		}
		for _, hit := range hits {
			pool[hit.Observation.ID] = hit.Observation
		}
	}
	observations := make([]model.AgentObservation, 0, len(pool))
	for _, obs := range pool {
		observations = append(observations, obs)
	}
	sort.Slice(observations, func(i, j int) bool {
		if !observations[i].CreatedAt.Equal(observations[j].CreatedAt) {
			return observations[i].CreatedAt.Before(observations[j].CreatedAt)
		}
		return observations[i].ID.String() < observations[j].ID.String()
	})

	// 3. Map UUIDs to small integer strings before the LLM sees anything.
	factIDs := newConsolidationIDMap()
	observationIDs := newConsolidationIDMap()
	userPrompt, err := buildConsolidationUserPrompt(facts, observations, factIDs, observationIDs)
	if err != nil {
		return err
	}

	// 4. One LLM call, temperature 0.0, strict JSON.
	raw, err := h.completion()(ctx, consolidationSystemPrompt, userPrompt,
		"memory_consolidation", json.RawMessage(consolidationResponseSchema), memoryConsolidationMaxTokens)
	if err != nil {
		return fmt.Errorf("consolidation completion: %w", err)
	}
	ops, err := parseConsolidationResponse(raw)
	if err != nil {
		return err
	}
	resolved := resolveConsolidationOps(ops, factIDs, observationIDs)
	if resolved.Skipped > 0 {
		logger.WarnContext(ctx, "consolidation ops referencing unknown ids skipped",
			"org_id", payload.OrgID, "channel_id", payload.ChannelID, "skipped", resolved.Skipped)
	}

	factByID := make(map[uuid.UUID]model.AgentMemory, len(facts))
	for _, fact := range facts {
		factByID[fact.ID] = fact
	}

	// 5. Apply ops.
	changed := make([]uuid.UUID, 0, len(resolved.Creates)+len(resolved.Updates))
	for _, create := range resolved.Creates {
		obs, created, err := h.applyCreate(ctx, svc, payload, create, factByID, now)
		if err != nil {
			return err
		}
		if created {
			changed = append(changed, obs.ID)
		}
	}
	for _, update := range resolved.Updates {
		obsID, applied, err := h.applyUpdate(ctx, svc, payload.OrgID, update, now)
		if err != nil {
			return err
		}
		if applied {
			changed = append(changed, obsID)
		}
	}
	for _, del := range resolved.Deletes {
		if err := h.applyDelete(ctx, svc, payload.OrgID, del, now); err != nil {
			return err
		}
	}

	// Mark the whole batch consolidated: unreferenced facts were judged
	// ephemeral by the consolidator and are intentionally dropped.
	allFactIDs := make([]uuid.UUID, 0, len(facts))
	for _, fact := range facts {
		allFactIDs = append(allFactIDs, fact.ID)
	}
	if err := svc.MarkFactsConsolidated(ctx, payload.OrgID, allFactIDs, now); err != nil {
		return err
	}

	// 6+7. Embed changed observations and run the semantic dedup post-pass.
	h.embedAndDedup(ctx, svc, payload.OrgID, changed, now)

	// Digest recompute after every run (zero-latency recall contract).
	if err := svc.RecomputeChannelDigest(ctx, payload.OrgID, payload.ChannelID); err != nil {
		return err
	}

	// More unconsolidated facts than one batch: chain the next run.
	if len(facts) == memoryConsolidationBatchSize {
		if err := EnqueueMemoryConsolidate(ctx, h.enqueuer, payload.OrgID, payload.ChannelID); err != nil {
			logger.WarnContext(ctx, "enqueue follow-up consolidation failed", "error", err)
		}
	}
	return nil
}

func (h *MemoryConsolidationHandler) applyCreate(
	ctx context.Context,
	svc *memory.Service,
	payload MemoryConsolidatePayload,
	create resolvedCreate,
	factByID map[uuid.UUID]model.AgentMemory,
	now time.Time,
) (*model.AgentObservation, bool, error) {
	logger := logging.FromContext(ctx)
	humanSource := false
	var occurredStart, occurredEnd *time.Time
	for _, factID := range create.SourceFactIDs {
		fact, ok := factByID[factID]
		if !ok {
			continue
		}
		if factFromHumanActor(fact) {
			humanSource = true
		}
		at := fact.CreatedAt
		if occurredStart == nil || at.Before(*occurredStart) {
			start := at
			occurredStart = &start
		}
		if occurredEnd == nil || at.After(*occurredEnd) {
			end := at
			occurredEnd = &end
		}
	}
	obsChannelID := consolidationCreateChannelID(create.Op.Scope, payload.ChannelID, len(create.SourceFactIDs), humanSource)
	suppressed, err := svc.IsSuppressed(ctx, payload.OrgID, obsChannelID, create.Op.Text)
	if err != nil {
		return nil, false, err
	}
	if suppressed {
		logger.InfoContext(ctx, "consolidation create dropped by suppression list",
			"org_id", payload.OrgID, "channel_id", payload.ChannelID, "reason", create.Op.Reason)
		return nil, false, nil
	}
	kind := create.Op.Kind
	if !memory.ValidObservationKind(kind) {
		kind = "finding"
	}
	metadata := appendObservationAudit(model.JSON{"source": "consolidation"}, "create", create.Op.Reason, create.SourceFactIDs, now)
	obs, err := svc.CreateObservation(ctx, memory.CreateObservationRequest{
		OrgID:           payload.OrgID,
		ChannelID:       obsChannelID,
		Content:         create.Op.Text,
		Kind:            kind,
		Entities:        create.Op.Entities,
		SourceFactIDs:   create.SourceFactIDs,
		OccurredStart:   occurredStart,
		OccurredEnd:     occurredEnd,
		LastMentionedAt: now,
		ExpiresAt:       parseConsolidationExpiresAt(create.Op.ExpiresAt),
		Metadata:        metadata,
	})
	if err != nil {
		return nil, false, err
	}
	return obs, true, nil
}

func (h *MemoryConsolidationHandler) applyUpdate(
	ctx context.Context,
	svc *memory.Service,
	orgID uuid.UUID,
	update resolvedUpdate,
	now time.Time,
) (uuid.UUID, bool, error) {
	obs, err := svc.GetObservation(ctx, orgID, update.ObservationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logging.FromContext(ctx).WarnContext(ctx, "consolidation update target missing",
			"observation_id", update.ObservationID)
		return uuid.Nil, false, nil
	}
	if err != nil {
		return uuid.Nil, false, err
	}
	contentChanged := applyConsolidationUpdate(obs, update.Op.Text, update.Op.Reason, update.SourceFactIDs, now)
	if err := svc.SaveObservationChanges(ctx, obs, contentChanged); err != nil {
		return uuid.Nil, false, err
	}
	return obs.ID, true, nil
}

func (h *MemoryConsolidationHandler) applyDelete(
	ctx context.Context,
	svc *memory.Service,
	orgID uuid.UUID,
	del resolvedDelete,
	now time.Time,
) error {
	obs, err := svc.GetObservation(ctx, orgID, del.ObservationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		logging.FromContext(ctx).WarnContext(ctx, "consolidation delete target missing",
			"observation_id", del.ObservationID)
		return nil
	}
	if err != nil {
		return err
	}
	obs.Metadata = appendObservationAudit(obs.Metadata, "delete", del.Op.Reason, nil, now)
	if err := svc.SaveObservationChanges(ctx, obs, false); err != nil {
		return err
	}
	if err := svc.ArchiveObservation(ctx, orgID, obs.ID, nil); err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	return nil
}

// embedAndDedup embeds each created/updated observation synchronously (the
// dedup probe needs the vector right away; failures fall back to the async
// observation embed queue) and merges near-identical same-scope pairs via the
// tiny merge/keep adjudicator.
func (h *MemoryConsolidationHandler) embedAndDedup(
	ctx context.Context,
	svc *memory.Service,
	orgID uuid.UUID,
	changed []uuid.UUID,
	now time.Time,
) {
	logger := logging.FromContext(ctx)
	for _, obsID := range changed {
		obs, err := svc.GetObservation(ctx, orgID, obsID)
		if err != nil {
			// Archived by an earlier dedup merge in this same pass, or gone.
			continue
		}
		vector, err := h.embedObservation(ctx, svc, obs)
		if err != nil {
			logger.WarnContext(ctx, "observation embed failed; queued for retry",
				"observation_id", obs.ID, "error", err)
			continue
		}
		if err := h.dedupObservation(ctx, svc, obs, vector, now); err != nil {
			logger.WarnContext(ctx, "observation semantic dedup failed",
				"observation_id", obs.ID, "error", err)
		}
	}
}

func (h *MemoryConsolidationHandler) embedObservation(
	ctx context.Context,
	svc *memory.Service,
	obs *model.AgentObservation,
) ([]float32, error) {
	vectors, err := svc.EmbedContents(ctx, []string{obs.Content})
	if err != nil {
		if enqueueErr := EnqueueObservationEmbed(ctx, h.enqueuer, obs.ID, obs.EmbeddingRevision); enqueueErr != nil {
			return nil, errors.Join(err, enqueueErr)
		}
		return nil, err
	}
	if _, err := svc.MarkObservationEmbeddingReady(ctx, obs.ID, obs.EmbeddingRevision, vectors[0]); err != nil {
		return nil, err
	}
	return vectors[0], nil
}

func (h *MemoryConsolidationHandler) dedupObservation(
	ctx context.Context,
	svc *memory.Service,
	obs *model.AgentObservation,
	vector []float32,
	now time.Time,
) error {
	obsID := obs.ID
	hits, err := svc.SimilarObservations(ctx, memory.SimilarObservationsRequest{
		OrgID:     obs.OrgID,
		ChannelID: obs.ChannelID, // nil probes org-wide scope only — same scope as obs
		Vector:    vector,
		Limit:     memoryConsolidationTopK,
		ExcludeID: &obsID,
	})
	if err != nil {
		return err
	}
	if len(hits) == 0 || hits[0].Similarity < memoryConsolidationDedupThreshold {
		return nil
	}
	twin := hits[0].Observation
	prompt := fmt.Sprintf(consolidationDedupPrompt, obs.Content, twin.Content)
	raw, err := h.completion()(ctx, "", prompt,
		"memory_dedup", json.RawMessage(consolidationDedupResponseSchema), memoryConsolidationDedupMaxTokens)
	if err != nil {
		return err
	}
	decision, err := parseConsolidationDedupResponse(raw)
	if err != nil {
		return err
	}
	if decision.Action != "merge" {
		return nil
	}
	return h.mergeObservations(ctx, svc, obs, &twin, decision, now)
}

// mergeObservations keeps the older row, merges text, sums proof counts,
// unions source facts, and archives the newer row with superseded_by.
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
type MemoryConsolidationSweepHandler struct {
	db       *gorm.DB
	enqueuer enqueue.TaskEnqueuer
}

func NewMemoryConsolidationSweepHandler(db *gorm.DB, enqueuer enqueue.TaskEnqueuer) *MemoryConsolidationSweepHandler {
	return &MemoryConsolidationSweepHandler{db: db, enqueuer: enqueuer}
}

func (h *MemoryConsolidationSweepHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.db == nil || h.enqueuer == nil {
		return nil
	}
	svc := memory.NewService(memory.Config{DB: h.db})
	channels, err := svc.ChannelsWithUnconsolidatedFacts(ctx, memoryConsolidationSweepLimit)
	if err != nil {
		return err
	}
	for _, ch := range channels {
		if err := EnqueueMemoryConsolidate(ctx, h.enqueuer, ch.OrgID, ch.ChannelID); err != nil {
			return fmt.Errorf("enqueue memory consolidation: %w", err)
		}
	}
	return nil
}

// MemoryObservationExpireHandler is the nightly lifecycle job: archive
// observations whose expires_at has passed, then refresh every affected
// channel memory digest.
type MemoryObservationExpireHandler struct {
	db           *gorm.DB
	cacheManager *cache.Manager
	memoryCfg    MemoryEmbeddingConfig
	now          func() time.Time
}

func NewMemoryObservationExpireHandler(db *gorm.DB, cacheManager *cache.Manager, memoryCfg MemoryEmbeddingConfig) *MemoryObservationExpireHandler {
	return &MemoryObservationExpireHandler{db: db, cacheManager: cacheManager, memoryCfg: memoryCfg}
}

func (h *MemoryObservationExpireHandler) Handle(ctx context.Context, _ *asynq.Task) error {
	if h == nil || h.db == nil {
		return nil
	}
	now := time.Now().UTC()
	if h.now != nil {
		now = h.now().UTC()
	}
	svc := memory.NewService(memory.Config{
		DB:             h.db,
		CacheManager:   h.cacheManager,
		EmbeddingModel: h.memoryCfg.Model,
		EmbeddingDim:   h.memoryCfg.Dim,
	})
	expired, err := svc.ExpireObservations(ctx, now)
	if err != nil {
		return err
	}
	if len(expired) == 0 {
		return nil
	}
	affected := map[uuid.UUID]uuid.UUID{} // channel -> org
	for _, obs := range expired {
		if obs.ChannelID != nil {
			affected[*obs.ChannelID] = obs.OrgID
			continue
		}
		// Org-wide expiry touches every channel digest that folds org
		// observations in; refresh all digests recorded for the org.
		var digests []model.ChannelMemoryDigest
		if err := h.db.WithContext(ctx).
			Where("org_id = ?", obs.OrgID).
			Find(&digests).Error; err != nil {
			return fmt.Errorf("list org digests for expiry: %w", err)
		}
		for _, digest := range digests {
			affected[digest.ChannelID] = digest.OrgID
		}
	}
	logger := logging.FromContext(ctx)
	for channelID, orgID := range affected {
		if err := svc.RecomputeChannelDigest(ctx, orgID, channelID); err != nil {
			logger.WarnContext(ctx, "digest recompute after expiry failed",
				"org_id", orgID, "channel_id", channelID, "error", err)
		}
	}
	logger.InfoContext(ctx, "expired observations archived",
		"count", len(expired), "digests_refreshed", len(affected))
	return nil
}

// ObservationEmbedHandler retries observation embeddings that failed during
// the consolidation worker's synchronous pass.
type ObservationEmbedHandler struct {
	db           *gorm.DB
	cacheManager *cache.Manager
	cfg          MemoryEmbeddingConfig
}

func NewObservationEmbedHandler(db *gorm.DB, cacheManager *cache.Manager, cfg MemoryEmbeddingConfig) *ObservationEmbedHandler {
	return &ObservationEmbedHandler{db: db, cacheManager: cacheManager, cfg: cfg}
}

func (h *ObservationEmbedHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload ObservationEmbedPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal observation embed payload: %w", err)
	}
	svc := memory.NewService(memory.Config{
		DB:             h.db,
		CacheManager:   h.cacheManager,
		EmbeddingModel: h.cfg.Model,
		EmbeddingDim:   h.cfg.Dim,
	})
	obs, err := svc.LoadObservation(ctx, payload.ObservationID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	if obs.ArchivedAt != nil || obs.EmbeddingRevision != payload.Revision ||
		obs.EmbeddingStatus == model.AgentMemoryEmbeddingReady {
		return nil
	}
	vectors, err := svc.EmbedContents(ctx, []string{obs.Content})
	if err != nil {
		_ = svc.MarkObservationEmbeddingFailed(ctx, obs.ID, payload.Revision, err)
		return err
	}
	if _, err := svc.MarkObservationEmbeddingReady(ctx, obs.ID, payload.Revision, vectors[0]); err != nil {
		return err
	}
	return nil
}
