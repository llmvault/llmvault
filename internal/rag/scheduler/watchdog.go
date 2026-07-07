package scheduler

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

// ScanStuckAttempts is the only crash-recovery path: without it, a
// worker SIGKILL leaves the attempt row IN_PROGRESS and the ingest scan
// predicate (which excludes sources with an in-flight attempt) will
// never re-enqueue.
func ScanStuckAttempts(ctx context.Context, db *gorm.DB, cfg Config) (int, error) {
	cutoff := time.Now().Add(-cfg.WatchdogTimeout)
	const errMsg = "watchdog: attempt exceeded heartbeat timeout"

	// Identify the stuck attempts first so we can report which sources they
	// belong to — an orphaned attempt is otherwise silent until this sweep.
	var stuck []struct {
		ID          uuid.UUID
		RagSourceID uuid.UUID
	}
	if err := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("status = ?", ragmodel.IndexingStatusInProgress).
		Where("(last_progress_time IS NOT NULL AND last_progress_time < ?) OR "+
			"(last_progress_time IS NULL AND time_created < ?)", cutoff, cutoff).
		Select("id", "rag_source_id").
		Scan(&stuck).Error; err != nil {
		return 0, fmt.Errorf("watchdog: scan stuck attempts: %w", err)
	}
	if len(stuck) == 0 {
		return 0, nil
	}

	ids := make([]uuid.UUID, len(stuck))
	sourceIDs := make([]string, len(stuck))
	for i, s := range stuck {
		ids[i] = s.ID
		sourceIDs[i] = s.RagSourceID.String()
	}

	res := db.WithContext(ctx).
		Model(&ragmodel.RAGIndexAttempt{}).
		Where("id IN ?", ids).
		Updates(map[string]any{
			"status":       ragmodel.IndexingStatusFailed,
			"error_msg":    errMsg,
			"time_updated": time.Now(),
		})
	if res.Error != nil {
		return 0, fmt.Errorf("watchdog: update stuck attempts: %w", res.Error)
	}

	logging.CaptureWithFields(ctx,
		fmt.Errorf("watchdog reclaimed %d stuck ingest attempt(s)", len(ids)),
		map[string]any{"count": len(ids), "source_ids": sourceIDs})

	return int(res.RowsAffected), nil
}
