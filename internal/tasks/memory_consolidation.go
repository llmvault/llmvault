package tasks

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
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
