package tasks

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
)

type SessionEventsNoticePublisher interface {
	PublishSessionEventsAppended(ctx context.Context, orgID, sessionID uuid.UUID, eventID string, eventAt time.Time) error
}

func publishSessionEventsAppended(ctx context.Context, notices SessionEventsNoticePublisher, orgID, sessionID uuid.UUID, eventID string, eventAt time.Time) {
	if notices == nil {
		return
	}
	if err := notices.PublishSessionEventsAppended(ctx, orgID, sessionID, eventID, eventAt); err != nil {
		logging.Capture(ctx, fmt.Errorf("publish session.events.appended notice: %w", err))
	}
}
