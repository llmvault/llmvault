package handler

import (
	"context"
	"fmt"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const agentEventBatchSize = 100

// agentEventFlushRetries bounds drain re-attempts before giving up. A transient
// Postgres blip must not silently drop session events (notably final).
const agentEventFlushRetries = 5

const (
	agentEventFlushBackoff    = 250 * time.Millisecond
	agentEventFlushBackoffMax = 5 * time.Second
	// agentEventShutdownFlushTimeout bounds the final flush so a wedged DB
	// cannot hang process shutdown indefinitely.
	agentEventShutdownFlushTimeout = 25 * time.Second
)

type afterWriteFn func(context.Context, []model.SessionEvent)

type AgentEventWriter struct {
	db            *gorm.DB
	entries       chan model.SessionEvent
	wg            sync.WaitGroup
	flushInterval time.Duration
	// afterWrite is set (via SetAfterWrite) after the drain goroutine starts, so
	// atomic.Pointer synchronises the publish to drain without a data race.
	afterWrite atomic.Pointer[afterWriteFn]
}

func NewAgentEventWriter(ctx context.Context, db *gorm.DB, bufferSize int, flushInterval ...time.Duration) *AgentEventWriter {
	interval := 500 * time.Millisecond
	if len(flushInterval) > 0 {
		interval = flushInterval[0]
	}
	w := &AgentEventWriter{
		db:            db,
		entries:       make(chan model.SessionEvent, bufferSize),
		flushInterval: interval,
	}
	w.wg.Add(1)
	goroutine.Go(ctx, func(ctx context.Context) {
		w.drain(ctx)
	})
	return w
}

func (w *AgentEventWriter) SetAfterWrite(fn func(context.Context, []model.SessionEvent)) {
	if w == nil {
		return
	}
	if fn == nil {
		w.afterWrite.Store(nil)
		return
	}
	cb := afterWriteFn(fn)
	w.afterWrite.Store(&cb)
}

func (w *AgentEventWriter) loadAfterWrite() afterWriteFn {
	if cb := w.afterWrite.Load(); cb != nil {
		return *cb
	}
	return nil
}

func (w *AgentEventWriter) drain(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "agent event drain panicked",
				"panic", r,
				"stack", string(debug.Stack()),
			)
			logging.CaptureWithFields(ctx, fmt.Errorf("agent event drain panicked: %v", r), map[string]any{
				"stage": "agent_event_drain_panic",
			})
		}
		w.wg.Done()
	}()

	batch := make([]model.SessionEvent, 0, agentEventBatchSize)
	timer := time.NewTimer(w.flushInterval)
	defer timer.Stop()

	// flush persists the current batch with bounded retries. On shutdown the
	// caller passes a non-cancelled flushCtx so the final flush is not aborted by
	// the cancelled root signal context.
	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if w.flushBatch(flushCtx, batch) {
			if cb := w.loadAfterWrite(); cb != nil {
				cb(flushCtx, append([]model.SessionEvent(nil), batch...))
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-w.entries:
			if !ok {
				// Shutdown: root ctx cancelled, so persist the buffer on a detached ctx.
				flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), agentEventShutdownFlushTimeout)
				flush(flushCtx)
				cancel()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= agentEventBatchSize {
				flush(ctx)
				if !timer.Stop() {
					select {
					case <-timer.C:
					default:
					}
				}
				timer.Reset(w.flushInterval)
			}
		case <-timer.C:
			flush(ctx)
			timer.Reset(w.flushInterval)
		}
	}
}

// flushBatch inserts batch, retrying transient failures with capped backoff.
// It reports whether the batch was durably persisted. Schedule-sync failures are
// non-fatal (logged) and do not fail the batch.
func (w *AgentEventWriter) flushBatch(ctx context.Context, batch []model.SessionEvent) bool {
	backoff := agentEventFlushBackoff
	var lastErr error
	for attempt := 0; attempt < agentEventFlushRetries; attempt++ {
		if attempt > 0 {
			if !sleepCtx(ctx, backoff) {
				// Context cancelled mid-retry: stop retrying and report failure.
				break
			}
			backoff *= 2
			if backoff > agentEventFlushBackoffMax {
				backoff = agentEventFlushBackoffMax
			}
		}
		err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(sessionEventOnConflict()).CreateInBatches(batch, agentEventBatchSize).Error; err != nil {
				return err
			}
			for _, entry := range batch {
				if err := syncAgentScheduleEvent(tx, entry); err != nil {
					captureSessionEventFailure(ctx, "batch_sync_schedule", entry, err)
					logging.FromContext(ctx).WarnContext(ctx, "agent schedule sync failed",
						"error", err,
						"event_type", entry.EventType,
						"agent_id", entry.AgentID,
					)
				}
			}
			return nil
		})
		if err == nil {
			return true
		}
		// A unique-violation means a prior attempt already persisted these rows.
		if isDuplicateKeyError(err) {
			return true
		}
		lastErr = err
		logging.FromContext(ctx).WarnContext(ctx, "agent event batch write retrying",
			"error", err, "attempt", attempt+1, "count", len(batch))
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("agent event batch write failed: %w", lastErr), agentEventBatchSentryFields("batch_write", batch))
	logging.FromContext(ctx).ErrorContext(ctx, "agent event batch write failed after retries", "error", lastErr, "count", len(batch))
	return false
}

func sleepCtx(ctx context.Context, d time.Duration) bool {
	timer := time.NewTimer(d)
	defer timer.Stop()
	select {
	case <-timer.C:
		return true
	case <-ctx.Done():
		return false
	}
}

func (w *AgentEventWriter) Write(ctx context.Context, entry model.SessionEvent) {
	if w == nil {
		return
	}
	select {
	case w.entries <- entry:
	default:
		err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(sessionEventOnConflict()).Create(&entry).Error; err != nil {
				return err
			}
			if err := syncAgentScheduleEvent(tx, entry); err != nil {
				captureSessionEventFailure(ctx, "direct_sync_schedule", entry, err)
				logging.FromContext(ctx).WarnContext(ctx, "agent schedule sync failed",
					"error", err,
					"event_type", entry.EventType,
					"agent_id", entry.AgentID,
				)
			}
			return nil
		})
		if err != nil {
			captureSessionEventFailure(ctx, "direct_write", entry, err)
			logging.FromContext(ctx).ErrorContext(ctx, "agent event direct write failed", "error", err, "event_type", entry.EventType)
		} else if cb := w.loadAfterWrite(); cb != nil {
			cb(ctx, []model.SessionEvent{entry})
		}
	}
}

func (w *AgentEventWriter) Shutdown(ctx context.Context) {
	if w == nil {
		return
	}
	close(w.entries)

	done := make(chan struct{})
	goroutine.Go(ctx, func(context.Context) {
		w.wg.Wait()
		close(done)
	})

	select {
	case <-done:
	case <-ctx.Done():
		logging.FromContext(ctx).WarnContext(ctx, "agent event shutdown timed out, some entries may be lost")
	}
}

func agentEventBatchSentryFields(stage string, batch []model.SessionEvent) map[string]any {
	fields := map[string]any{
		"stage": stage,
		"count": len(batch),
	}
	if len(batch) == 0 {
		return fields
	}
	first := batch[0]
	fields["org_id"] = first.OrgID.String()
	fields["agent_id"] = first.AgentID.String()
	fields["sandbox_id"] = first.SandboxID.String()
	fields["session_id"] = first.SessionID
	fields["event_type"] = first.EventType
	fields["source"] = first.Source
	return fields
}
