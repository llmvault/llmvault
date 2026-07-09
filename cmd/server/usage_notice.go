package main

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/runtimestream"
)

type runtimeStreamUsageNoticePublisher struct {
	store *runtimestream.Store
}

func newRuntimeStreamUsageNoticePublisher(store *runtimestream.Store) *runtimeStreamUsageNoticePublisher {
	return &runtimeStreamUsageNoticePublisher{store: store}
}

func (p *runtimeStreamUsageNoticePublisher) PublishUsageUpdated(ctx context.Context, orgID, sessionID uuid.UUID) error {
	if p == nil || p.store == nil {
		return nil
	}
	data, err := json.Marshal(map[string]string{"session_id": sessionID.String()})
	if err != nil {
		return fmt.Errorf("marshal usage.updated payload: %w", err)
	}
	return p.store.PublishNotice(ctx, sessionID, runtimestream.Notice{
		Type:      runtimestream.NoticeTypeUsageUpdated,
		OrgID:     orgID,
		SessionID: &sessionID,
		Data:      data,
	})
}
