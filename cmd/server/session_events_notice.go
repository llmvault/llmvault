package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/runtimestream"
)

type runtimeStreamSessionEventsNoticePublisher struct {
	store *runtimestream.Store
}

func newRuntimeStreamSessionEventsNoticePublisher(store *runtimestream.Store) *runtimeStreamSessionEventsNoticePublisher {
	return &runtimeStreamSessionEventsNoticePublisher{store: store}
}

func (p *runtimeStreamSessionEventsNoticePublisher) PublishSessionEventsAppended(ctx context.Context, orgID, sessionID uuid.UUID, eventID string, eventAt time.Time) error {
	if p == nil || p.store == nil {
		return nil
	}
	data, err := json.Marshal(map[string]string{
		"event_id": eventID,
		"event_at": eventAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return fmt.Errorf("marshal session.events.appended payload: %w", err)
	}
	return p.store.PublishNotice(ctx, sessionID, runtimestream.Notice{
		Type:      runtimestream.NoticeTypeSessionEventsAppended,
		OrgID:     orgID,
		SessionID: &sessionID,
		Data:      data,
	})
}
