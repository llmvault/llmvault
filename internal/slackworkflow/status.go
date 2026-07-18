package slackworkflow

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

func MarkStatusSet(ctx context.Context, db *gorm.DB, id uuid.UUID) {
	markTime(ctx, db, id, map[string]any{"status_set_at": time.Now().UTC()})
}

func MarkEnqueued(ctx context.Context, db *gorm.DB, id uuid.UUID) error {
	return updateEvent(ctx, db, id, map[string]any{
		"status":      model.SlackThreadEventStatusEnqueued,
		"enqueued_at": time.Now().UTC(),
		"error":       "",
	})
}

func MarkProcessing(ctx context.Context, db *gorm.DB, id uuid.UUID) error {
	return updateEvent(ctx, db, id, map[string]any{
		"status":         model.SlackThreadEventStatusProcessing,
		"job_started_at": time.Now().UTC(),
		"error":          "",
	})
}

func MarkFailed(ctx context.Context, db *gorm.DB, id uuid.UUID, err error) {
	msg := ""
	if err != nil {
		msg = truncate(err.Error(), 1000)
	}
	_ = updateEvent(ctx, db, id, map[string]any{
		"status":    model.SlackThreadEventStatusFailed,
		"failed_at": time.Now().UTC(),
		"error":     msg,
	})
}

func RecordRouteResolved(ctx context.Context, db *gorm.DB, id, teamID, agentID uuid.UUID) error {
	return updateEvent(ctx, db, id, map[string]any{
		"team_id":           &teamID,
		"agent_id":          &agentID,
		"route_resolved_at": time.Now().UTC(),
	})
}

func RecordSessionResolved(ctx context.Context, db *gorm.DB, id, sessionID uuid.UUID) error {
	return updateEvent(ctx, db, id, map[string]any{
		"session_id":          &sessionID,
		"session_resolved_at": time.Now().UTC(),
	})
}

func RecordRuntimePosted(ctx context.Context, db *gorm.DB, id uuid.UUID, streamID, turnID string) error {
	return updateEvent(ctx, db, id, map[string]any{
		"runtime_stream_id": strings.TrimSpace(streamID),
		"runtime_turn_id":   strings.TrimSpace(turnID),
		"runtime_posted_at": time.Now().UTC(),
	})
}

func RecordFinalReceived(ctx context.Context, db *gorm.DB, id uuid.UUID) {
	markTime(ctx, db, id, map[string]any{"final_received_at": time.Now().UTC()})
}

func RecordReplySent(ctx context.Context, db *gorm.DB, id uuid.UUID, replyTS string) error {
	now := time.Now().UTC()
	return updateEvent(ctx, db, id, map[string]any{
		"status":              model.SlackThreadEventStatusCompleted,
		"slack_reply_ts":      strings.TrimSpace(replyTS),
		"slack_reply_sent_at": now,
		"completed_at":        now,
		"error":               "",
	})
}

func updateEvent(ctx context.Context, db *gorm.DB, id uuid.UUID, updates map[string]any) error {
	if db == nil || id == uuid.Nil {
		return nil
	}
	updates["updated_at"] = time.Now().UTC()
	if err := db.WithContext(ctx).Model(&model.SlackThreadEvent{}).
		Where("id = ?", id).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("update slack thread event: %w", err)
	}
	return nil
}

func markTime(ctx context.Context, db *gorm.DB, id uuid.UUID, updates map[string]any) {
	_ = updateEvent(ctx, db, id, updates)
}

func truncate(value string, limit int) string {
	if len(value) <= limit {
		return value
	}
	return value[:limit]
}
