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

const employeeEventBatchSize = 100

// employeeEventFlushRetries bounds how many times the drain re-attempts a failed
// batch insert before giving up. A transient Postgres blip must not silently
// drop session events (notably agent.message.sent, the only durable record of a
// reply), so we retry with backoff before logging/capturing a hard loss.
const employeeEventFlushRetries = 5

const (
	employeeEventFlushBackoff    = 250 * time.Millisecond
	employeeEventFlushBackoffMax = 5 * time.Second
	// employeeEventShutdownFlushTimeout bounds the final flush so a wedged DB
	// cannot hang process shutdown indefinitely.
	employeeEventShutdownFlushTimeout = 25 * time.Second
)

type afterWriteFn func(context.Context, []model.EmployeeSessionEvent)

type EmployeeEventWriter struct {
	db            *gorm.DB
	entries       chan model.EmployeeSessionEvent
	wg            sync.WaitGroup
	flushInterval time.Duration
	// afterWrite is set after the drain goroutine has already started (via
	// SetAfterWrite, wired during handler setup), so the write/read must be
	// synchronised. atomic.Pointer makes the publish visible to drain without
	// a data race (P2-39).
	afterWrite atomic.Pointer[afterWriteFn]
}

func NewEmployeeEventWriter(ctx context.Context, db *gorm.DB, bufferSize int, flushInterval ...time.Duration) *EmployeeEventWriter {
	interval := 500 * time.Millisecond
	if len(flushInterval) > 0 {
		interval = flushInterval[0]
	}
	w := &EmployeeEventWriter{
		db:            db,
		entries:       make(chan model.EmployeeSessionEvent, bufferSize),
		flushInterval: interval,
	}
	w.wg.Add(1)
	go w.drain(ctx)
	return w
}

func (w *EmployeeEventWriter) SetAfterWrite(fn func(context.Context, []model.EmployeeSessionEvent)) {
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

// loadAfterWrite returns the currently-registered afterWrite callback, or nil.
func (w *EmployeeEventWriter) loadAfterWrite() afterWriteFn {
	if cb := w.afterWrite.Load(); cb != nil {
		return *cb
	}
	return nil
}

func (w *EmployeeEventWriter) drain(ctx context.Context) {
	defer func() {
		if r := recover(); r != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "employee event drain panicked",
				"panic", r,
				"stack", string(debug.Stack()),
			)
			logging.CaptureWithFields(ctx, fmt.Errorf("employee event drain panicked: %v", r), map[string]any{
				"stage": "employee_event_drain_panic",
			})
		}
		w.wg.Done()
	}()

	batch := make([]model.EmployeeSessionEvent, 0, employeeEventBatchSize)
	timer := time.NewTimer(w.flushInterval)
	defer timer.Stop()

	// flush persists the current batch with bounded retries. flushCtx is the
	// context used for the DB write: during normal operation it is the drain's
	// long-lived ctx; on shutdown the caller passes a non-cancelled context so
	// the final flush is not aborted by the cancelled root signal context.
	flush := func(flushCtx context.Context) {
		if len(batch) == 0 {
			return
		}
		if w.flushBatch(flushCtx, batch) {
			if cb := w.loadAfterWrite(); cb != nil {
				cb(flushCtx, append([]model.EmployeeSessionEvent(nil), batch...))
			}
		}
		batch = batch[:0]
	}

	for {
		select {
		case entry, ok := <-w.entries:
			if !ok {
				// Shutdown: the root ctx is already cancelled, so persist the
				// remaining buffer on a detached, time-bounded context.
				flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), employeeEventShutdownFlushTimeout)
				flush(flushCtx)
				cancel()
				return
			}
			batch = append(batch, entry)
			if len(batch) >= employeeEventBatchSize {
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
func (w *EmployeeEventWriter) flushBatch(ctx context.Context, batch []model.EmployeeSessionEvent) bool {
	backoff := employeeEventFlushBackoff
	var lastErr error
	for attempt := 0; attempt < employeeEventFlushRetries; attempt++ {
		if attempt > 0 {
			if !sleepCtx(ctx, backoff) {
				// Context cancelled mid-retry: stop retrying and report failure.
				break
			}
			backoff *= 2
			if backoff > employeeEventFlushBackoffMax {
				backoff = employeeEventFlushBackoffMax
			}
		}
		err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(employeeSessionEventOnConflict()).CreateInBatches(batch, employeeEventBatchSize).Error; err != nil {
				return err
			}
			for _, entry := range batch {
				if err := syncEmployeeScheduleEvent(tx, entry); err != nil {
					captureEmployeeSessionEventFailure(ctx, "batch_sync_schedule", entry, err)
					logging.FromContext(ctx).WarnContext(ctx, "employee schedule sync failed",
						"error", err,
						"event_type", entry.EventType,
						"employee_id", entry.EmployeeID,
					)
				}
			}
			return nil
		})
		if err == nil {
			return true
		}
		// A unique-violation means a prior attempt (or retry) already durably
		// persisted these rows; treat it as success rather than a loss.
		if isDuplicateKeyError(err) {
			return true
		}
		lastErr = err
		logging.FromContext(ctx).WarnContext(ctx, "employee event batch write retrying",
			"error", err, "attempt", attempt+1, "count", len(batch))
	}
	logging.CaptureWithFields(ctx, fmt.Errorf("employee event batch write failed: %w", lastErr), employeeEventBatchSentryFields("batch_write", batch))
	logging.FromContext(ctx).ErrorContext(ctx, "employee event batch write failed after retries", "error", lastErr, "count", len(batch))
	return false
}

// sleepCtx sleeps for d unless ctx is cancelled first. It returns true if the
// full sleep elapsed.
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

func (w *EmployeeEventWriter) Write(ctx context.Context, entry model.EmployeeSessionEvent) {
	if w == nil {
		return
	}
	select {
	case w.entries <- entry:
	default:
		err := w.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if err := tx.Clauses(employeeSessionEventOnConflict()).Create(&entry).Error; err != nil {
				return err
			}
			if err := syncEmployeeScheduleEvent(tx, entry); err != nil {
				captureEmployeeSessionEventFailure(ctx, "direct_sync_schedule", entry, err)
				logging.FromContext(ctx).WarnContext(ctx, "employee schedule sync failed",
					"error", err,
					"event_type", entry.EventType,
					"employee_id", entry.EmployeeID,
				)
			}
			return nil
		})
		if err != nil {
			captureEmployeeSessionEventFailure(ctx, "direct_write", entry, err)
			logging.FromContext(ctx).ErrorContext(ctx, "employee event direct write failed", "error", err, "event_type", entry.EventType)
		} else if cb := w.loadAfterWrite(); cb != nil {
			cb(ctx, []model.EmployeeSessionEvent{entry})
		}
	}
}

func (w *EmployeeEventWriter) Shutdown(ctx context.Context) {
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
		logging.FromContext(ctx).WarnContext(ctx, "employee event shutdown timed out, some entries may be lost")
	}
}

func employeeEventBatchSentryFields(stage string, batch []model.EmployeeSessionEvent) map[string]any {
	fields := map[string]any{
		"stage": stage,
		"count": len(batch),
	}
	if len(batch) == 0 {
		return fields
	}
	first := batch[0]
	fields["org_id"] = first.OrgID.String()
	fields["employee_id"] = first.EmployeeID.String()
	fields["sandbox_id"] = first.SandboxID.String()
	fields["session_id"] = first.SessionID
	fields["event_type"] = first.EventType
	fields["source"] = first.Source
	return fields
}
