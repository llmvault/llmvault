package canvasartifact

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/runtimestream"
)

type ArtifactSyncedPayload struct {
	ArtifactID   string `json:"artifact_id"`
	ProjectID    string `json:"project_id"`
	Slug         string `json:"slug"`
	Name         string `json:"name"`
	ArtifactType string `json:"artifact_type"`
	Created      bool   `json:"created"`
}

type ArtifactSyncedNotice struct {
	OrgID     uuid.UUID
	SessionID uuid.UUID
	Payload   ArtifactSyncedPayload
}

// NoticePublisher delivers session notices emitted after a committed artifact
// sync. Implementations must be safe for concurrent use; a nil publisher on the
// Service disables publishing. Publish failures are swallowed by the
// implementation so a sync never fails because a notice could not be delivered.
type NoticePublisher interface {
	PublishArtifactSynced(ctx context.Context, notice ArtifactSyncedNotice)
}

// RuntimeNoticePublisher publishes artifact.synced notices to the session live
// channel via the runtimestream Store.
type RuntimeNoticePublisher struct {
	store *runtimestream.Store
}

func NewRuntimeNoticePublisher(store *runtimestream.Store) *RuntimeNoticePublisher {
	return &RuntimeNoticePublisher{store: store}
}

func (p *RuntimeNoticePublisher) PublishArtifactSynced(ctx context.Context, notice ArtifactSyncedNotice) {
	if p == nil || p.store == nil {
		return
	}
	data, err := json.Marshal(notice.Payload)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("canvasartifact: marshal notice: %w", err))
		return
	}
	sessionID := notice.SessionID
	if err := p.store.PublishNotice(ctx, sessionID, runtimestream.Notice{
		Type:      runtimestream.NoticeTypeArtifactSynced,
		OrgID:     notice.OrgID,
		SessionID: &sessionID,
		Data:      data,
	}); err != nil {
		logging.Capture(ctx, fmt.Errorf("canvasartifact: publish notice: %w", err))
	}
}
