package middleware

import (
	"context"
	"net"
	"net/http"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/observe"
	"github.com/usehivy/hivy/internal/providerheaders"
	"github.com/usehivy/hivy/internal/registry"
	"github.com/usehivy/hivy/internal/tasks"
)

const generationBatchSize = 50

// GenerationWriter is a buffered generation log writer that never blocks the request hot path.
type GenerationWriter struct {
	db            *gorm.DB
	reg           *registry.Registry
	enqueuer      enqueue.TaskEnqueuer
	entries       chan model.Generation
	wg            sync.WaitGroup
	flushInterval time.Duration
}

// NewGenerationWriter creates a GenerationWriter with the given buffer size and starts
// background flushing. Call Shutdown to flush remaining entries on exit.
// An optional flushInterval controls how often partial batches are flushed
// (default 500ms).
func NewGenerationWriter(ctx context.Context, db *gorm.DB, reg *registry.Registry, bufferSize int, flushInterval ...time.Duration) *GenerationWriter {
	interval := 500 * time.Millisecond
	if len(flushInterval) > 0 {
		interval = flushInterval[0]
	}
	gw := &GenerationWriter{
		db:            db,
		reg:           reg,
		entries:       make(chan model.Generation, bufferSize),
		flushInterval: interval,
	}
	gw.wg.Add(1)
	goroutine.Go(ctx, gw.drain)
	return gw
}

// SetEnqueuer wires a durable fallback: billing-bearing rows that can't be flushed
// (insert failure, full buffer, shutdown deadline) spill to asynq instead of dropping.
func (gw *GenerationWriter) SetEnqueuer(enqueuer enqueue.TaskEnqueuer) {
	gw.enqueuer = enqueuer
}

func (gw *GenerationWriter) drain(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "generation drain panicked",
				"panic", r,
				"stack", string(debug.Stack()),
			)
		}
		gw.wg.Done()
	}()

	batch := make([]model.Generation, 0, generationBatchSize)
	timer := time.NewTimer(gw.flushInterval)
	defer timer.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		gw.flushBatch(ctx, batch)
		batch = batch[:0]
	}

	for {
		select {
		case gen, ok := <-gw.entries:
			if !ok {
				flush()
				return
			}
			batch = append(batch, gen)
			if len(batch) >= generationBatchSize {
				flush()
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(gw.flushInterval)
			}
		case <-timer.C:
			flush()
			timer.Reset(gw.flushInterval)
		}
	}
}

// flushRetryDelays is the backoff before falling back to the durable asynq spill.
var flushRetryDelays = []time.Duration{50 * time.Millisecond, 200 * time.Millisecond}

// flushBatch writes the batch with a short bounded retry, spilling every row to
// the durable asynq task on persistent failure so billable usage is never
// dropped on a DB blip. The caller must not reuse batch after this returns.
func (gw *GenerationWriter) flushBatch(ctx context.Context, batch []model.Generation) {
	var err error
	for attempt := 0; ; attempt++ {
		err = gw.db.CreateInBatches(batch, generationBatchSize).Error
		if err == nil {
			return
		}
		if attempt >= len(flushRetryDelays) {
			break
		}
		select {
		case <-time.After(flushRetryDelays[attempt]):
		case <-ctx.Done():
			// Stop sleeping on shutdown, but still attempt the durable spill below.
		}
	}
	logging.FromContext(ctx).ErrorContext(ctx, "generation batch write failed, spilling to asynq",
		"error", err, "count", len(batch))
	for i := range batch {
		gw.spill(ctx, batch[i], "flush_failed")
	}
}

// spill enqueues a single generation row on the durable asynq generation:write
// task (MaxRetry 3). Only when the enqueuer is unset or itself fails is the
// billing-bearing row dropped — and that is logged loudly.
func (gw *GenerationWriter) spill(ctx context.Context, gen model.Generation, reason string) {
	if gw.enqueuer == nil {
		logging.FromContext(ctx).ErrorContext(ctx, "generation dropped (no spill enqueuer configured)",
			"id", gen.ID, "reason", reason)
		return
	}
	task, opts, err := tasks.NewGenerationWriteTask(gen)
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "generation spill marshal failed, dropping",
			"id", gen.ID, "reason", reason, "error", err)
		return
	}
	if _, err := gw.enqueuer.EnqueueContext(context.WithoutCancel(ctx), task, opts...); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "generation spill enqueue failed, dropping",
			"id", gen.ID, "reason", reason, "error", err)
	}
}

// Write queues a generation entry, never blocking the request hot path: a full
// buffer spills the billing-bearing entry to the durable asynq task.
func (gw *GenerationWriter) Write(ctx context.Context, gen model.Generation) {
	select {
	case gw.entries <- gen:
	default:
		logging.FromContext(ctx).WarnContext(ctx, "generation buffer full, spilling to asynq", "id", gen.ID)
		gw.spill(ctx, gen, "buffer_full")
	}
}

// Shutdown closes the channel and waits for all queued entries to be flushed.
// If the drain misses the deadline, queued entries spill to the durable asynq
// task rather than being abandoned.
func (gw *GenerationWriter) Shutdown(ctx context.Context) {
	close(gw.entries)

	done := make(chan struct{})
	goroutine.Go(ctx, func(context.Context) {
		gw.wg.Wait()
		close(done)
	})

	select {
	case <-done:
	case <-ctx.Done():
		logging.FromContext(ctx).WarnContext(ctx, "generation shutdown timed out, spilling remaining entries to asynq")
		// The drain raced the deadline; drain and spill the rest ourselves. The
		// channel is closed, so this ranges to completion safely.
		for gen := range gw.entries {
			gw.spill(context.WithoutCancel(ctx), gen, "shutdown_timeout")
		}
	}
}

func Generation(gw *GenerationWriter, db *gorm.DB, attrCache *AttributionCache) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := r.Context()
			claims, ok := ClaimsFromContext(ctx)
			if !ok {
				next.ServeHTTP(w, r)
				return
			}

			providerID := lookupProviderID(db, claims.CredentialID)

			captured := &observe.CapturedData{ProviderID: providerID, CredentialID: claims.CredentialID}
			r = r.WithContext(observe.WithCapturedData(ctx, captured))

			next.ServeHTTP(w, r)

			gen := buildGeneration(r, claims, captured, providerID, gw.reg, db, attrCache)
			gw.Write(ctx, gen)
		})
	}
}

func buildGeneration(r *http.Request, claims *TokenClaims, captured *observe.CapturedData, providerID string, reg *registry.Registry, db *gorm.DB, attrCache *AttributionCache) model.Generation {
	genID := "gen_" + ulid.Make().String()

	orgID, _ := uuid.Parse(claims.OrgID)
	credentialID := claims.CredentialID
	if captured.CredentialID != "" {
		credentialID = captured.CredentialID
	}
	credID, _ := uuid.Parse(credentialID)
	actualProviderID := captured.ProviderID
	if actualProviderID == "" {
		actualProviderID = providerID
	}
	if actualProviderID == "" {
		actualProviderID = lookupProviderID(db, credentialID)
	}

	gen := model.Generation{
		ID:           genID,
		OrgID:        orgID,
		CredentialID: credID,
		TokenJTI:     claims.JTI,
		ProviderID:   actualProviderID,
		Model:        captured.Model,
		RequestPath:  r.URL.Path,
		IsStreaming:  captured.IsStreaming,
		IsSystem:     claims.IsSystem,

		InputTokens:     captured.Usage.InputTokens,
		OutputTokens:    captured.Usage.OutputTokens,
		CachedTokens:    captured.Usage.CachedTokens,
		ReasoningTokens: captured.Usage.ReasoningTokens,

		TTFBMs:         &captured.TTFBMs,
		TotalMs:        captured.TotalMs,
		UpstreamStatus: captured.UpstreamStatus,

		ErrorType:    captured.ErrorType,
		ErrorMessage: truncateValidUTF8(captured.ErrorMessage, 1000),

		CreatedAt: time.Now().UTC(),
	}

	if captured.Usage.ProviderCostUSD > 0 {
		gen.Cost = captured.Usage.ProviderCostUSD
		gen.BillingCostSource = billing.CostSourceProvider
	} else {
		gen.Cost = calculateCost(reg, actualProviderID, captured.Model, captured.Usage)
	}
	if gen.Cost > 0 && gen.BillingCostSource == "" {
		gen.BillingCostSource = billing.CostSourceRegistry
	}

	if captured.GenerationID != "" && providerheaders.IsOpenRouter(actualProviderID, "") {
		id := captured.GenerationID
		gen.OpenRouterGenerationID = &id
	}

	extractAttribution(db, attrCache, claims.JTI, &gen)

	if ip, _, err := net.SplitHostPort(r.RemoteAddr); err == nil {
		gen.IPAddress = &ip
	} else {
		addr := r.RemoteAddr
		gen.IPAddress = &addr
	}

	return gen
}

// lookupProviderID fetches the credential's provider_id from the database.
func lookupProviderID(db *gorm.DB, credentialID string) string {
	var providerID string
	db.Model(&model.Credential{}).Select("provider_id").Where("id = ?", credentialID).Scan(&providerID)
	return providerID
}

func truncateValidUTF8(s string, maxLen int) string {
	s = strings.ToValidUTF8(s, "?")
	s = strings.ReplaceAll(s, "\x00", "?")
	if len(s) <= maxLen {
		return s
	}
	return strings.ToValidUTF8(s[:maxLen], "?")
}
