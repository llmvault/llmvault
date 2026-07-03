package sheets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

// Realtime event types published on a sheet's Redis channel (plan §2c).
const (
	EventRowsChanged       = "rows_changed"
	EventFieldsChanged     = "fields_changed"
	EventPagesChanged      = "pages_changed"
	EventImportProgress    = "import_progress"
	EventOperationReverted = "operation_reverted"
)

// maxEventPatchRows caps how many rows may carry inline cell patches in one
// rows_changed event; larger batches ship IDs only and clients refetch.
const maxEventPatchRows = 20

// maxMutationIDLen bounds the client-supplied echo token.
const maxMutationIDLen = 128

// LiveChannel is the Redis pub/sub channel for one sheet's realtime events.
// Braced UUID keeps all of a sheet's traffic on one cluster slot, matching
// runtimestream.LiveChannel.
func LiveChannel(sheetID uuid.UUID) string {
	return fmt.Sprintf("sheet:{%s}", sheetID)
}

// EventActor identifies who caused a change, for client-side attribution.
type EventActor struct {
	AgentID string `json:"agent_id,omitempty"`
	UserID  string `json:"user_id,omitempty"`
}

// EventRow is a full row snapshot carried by a rows_changed event for insert
// and update actions (apps live-stream contract). Apps consume these directly
// without a refetch; the web grid ignores the field. Snapshots ride only when
// the batch is within maxEventPatchRows — larger batches ship IDs only.
type EventRow struct {
	ID        string         `json:"id"`
	Data      map[string]any `json:"data"`
	Position  float64        `json:"position"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
}

// eventRowsFrom builds row snapshots for a rows_changed event.
func eventRowsFrom(rows []model.SheetRow) []EventRow {
	if len(rows) == 0 {
		return nil
	}
	out := make([]EventRow, 0, len(rows))
	for i := range rows {
		out = append(out, EventRow{
			ID:        rows[i].ID.String(),
			Data:      map[string]any(rows[i].Data),
			Position:  rows[i].Position,
			CreatedAt: rows[i].CreatedAt,
			UpdatedAt: rows[i].UpdatedAt,
		})
	}
	return out
}

// Event is the compact JSON payload published after a committed mutation.
// Events are hints — REST remains the source of truth (plan §2c).
type Event struct {
	Type          string                    `json:"type"`
	SheetID       string                    `json:"sheet_id"`
	PageID        string                    `json:"page_id,omitempty"`
	Action        string                    `json:"action,omitempty"`
	RowIDs        []string                  `json:"row_ids,omitempty"`
	Patches       map[string]map[string]any `json:"patches,omitempty"`
	FieldID       string                    `json:"field_id,omitempty"`
	OperationID   string                    `json:"operation_id,omitempty"`
	JobID         string                    `json:"job_id,omitempty"`
	ProcessedRows int64                     `json:"processed_rows,omitempty"`
	TotalRows     int64                     `json:"total_rows,omitempty"`
	Status        string                    `json:"status,omitempty"`
	Actor         *EventActor               `json:"actor,omitempty"`
	MutationID    string                    `json:"mutation_id,omitempty"`
	Rows          []EventRow                `json:"rows,omitempty"`
}

// EventPublisher delivers committed-mutation events. Implementations must be
// safe for concurrent use; a nil publisher on the Service disables publishing.
type EventPublisher interface {
	PublishSheetEvent(ctx context.Context, event Event)
}

// RedisEventPublisher publishes events to the sheet's LiveChannel.
type RedisEventPublisher struct {
	client *redis.Client
}

func NewRedisEventPublisher(client *redis.Client) *RedisEventPublisher {
	return &RedisEventPublisher{client: client}
}

func (p *RedisEventPublisher) PublishSheetEvent(ctx context.Context, event Event) {
	if p == nil || p.client == nil {
		return
	}
	sheetID, err := uuid.Parse(event.SheetID)
	if err != nil {
		return
	}
	raw, err := json.Marshal(event)
	if err != nil {
		logging.Capture(ctx, fmt.Errorf("sheets: marshal event: %w", err))
		return
	}
	if err := p.client.Publish(ctx, LiveChannel(sheetID), string(raw)).Err(); err != nil {
		logging.Capture(ctx, fmt.Errorf("sheets: publish event: %w", err))
	}
}

type mutationIDKey struct{}

// WithMutationID stores a client-supplied mutation identifier on the context;
// events emitted by mutations under this context echo it so clients can
// suppress their own writes.
func WithMutationID(ctx context.Context, id string) context.Context {
	id = strings.TrimSpace(id)
	if id == "" {
		return ctx
	}
	if len(id) > maxMutationIDLen {
		id = id[:maxMutationIDLen]
	}
	return context.WithValue(ctx, mutationIDKey{}, id)
}

// MutationIDFromContext returns the mutation identifier set by WithMutationID.
func MutationIDFromContext(ctx context.Context) string {
	id, _ := ctx.Value(mutationIDKey{}).(string)
	return id
}

func eventActor(actor Actor) *EventActor {
	out := &EventActor{}
	if actor.AgentID != nil {
		out.AgentID = actor.AgentID.String()
	}
	if actor.UserID != nil {
		out.UserID = actor.UserID.String()
	}
	if out.AgentID == "" && out.UserID == "" {
		return nil
	}
	return out
}

func (s *Service) emit(ctx context.Context, event Event) {
	if s.publisher == nil {
		return
	}
	if event.MutationID == "" {
		event.MutationID = MutationIDFromContext(ctx)
	}
	s.publisher.PublishSheetEvent(ctx, event)
}

// publishRowsChanged emits a rows_changed event. For insert and update actions
// it carries full row snapshots (rows) alongside the cell patches, under the
// same maxEventPatchRows cap: batches larger than the cap ship IDs only, both
// patches and snapshots dropped. Deletes pass rows=nil and stay IDs-only.
func (s *Service) publishRowsChanged(ctx context.Context, page *model.SheetPage, action string, rowIDs []string, patches map[string]map[string]any, rows []model.SheetRow, actor Actor) {
	if s.publisher == nil {
		return
	}
	var snapshots []EventRow
	if len(patches) > maxEventPatchRows {
		patches = nil
	} else {
		snapshots = eventRowsFrom(rows)
	}
	s.emit(ctx, Event{
		Type:    EventRowsChanged,
		SheetID: page.SheetID.String(),
		PageID:  page.ID.String(),
		Action:  action,
		RowIDs:  rowIDs,
		Patches: patches,
		Rows:    snapshots,
		Actor:   eventActor(actor),
	})
}

func (s *Service) publishFieldsChanged(ctx context.Context, orgID, pageID uuid.UUID, fieldID, action string, actor Actor) {
	if s.publisher == nil {
		return
	}
	sheetID, ok := s.sheetIDForPage(ctx, orgID, pageID)
	if !ok {
		return
	}
	s.emit(ctx, Event{
		Type:    EventFieldsChanged,
		SheetID: sheetID.String(),
		PageID:  pageID.String(),
		FieldID: fieldID,
		Action:  action,
		Actor:   eventActor(actor),
	})
}

func (s *Service) publishPagesChanged(ctx context.Context, sheetID, pageID uuid.UUID, action string) {
	if s.publisher == nil {
		return
	}
	s.emit(ctx, Event{
		Type:    EventPagesChanged,
		SheetID: sheetID.String(),
		PageID:  pageID.String(),
		Action:  action,
	})
}

func (s *Service) publishOperationReverted(ctx context.Context, orgID uuid.UUID, op *model.SheetOperation, actor Actor) {
	if s.publisher == nil {
		return
	}
	sheetID, ok := s.sheetIDForPage(ctx, orgID, op.PageID)
	if !ok {
		return
	}
	s.emit(ctx, Event{
		Type:        EventOperationReverted,
		SheetID:     sheetID.String(),
		PageID:      op.PageID.String(),
		OperationID: op.ID.String(),
		Actor:       eventActor(actor),
	})
}

// PublishImportProgress emits an import_progress event for a job. Exposed so
// the CSV import worker (Phase 4) can stream chunk progress.
func (s *Service) PublishImportProgress(ctx context.Context, job *model.SheetImportJob) {
	if s.publisher == nil || job == nil {
		return
	}
	sheetID, ok := s.sheetIDForPage(ctx, job.OrgID, job.PageID)
	if !ok {
		return
	}
	s.emit(ctx, Event{
		Type:          EventImportProgress,
		SheetID:       sheetID.String(),
		PageID:        job.PageID.String(),
		JobID:         job.ID.String(),
		ProcessedRows: job.ProcessedRows,
		TotalRows:     job.TotalRows,
		Status:        job.Status,
	})
}

// sheetIDForPage resolves a page's sheet for event routing. Archived pages
// still resolve so page-archive events reach subscribers.
func (s *Service) sheetIDForPage(ctx context.Context, orgID, pageID uuid.UUID) (uuid.UUID, bool) {
	var raw string
	err := s.db.WithContext(ctx).Model(&model.SheetPage{}).
		Where("id = ? AND org_id = ?", pageID, orgID).
		Limit(1).Pluck("sheet_id", &raw).Error
	if err != nil {
		return uuid.Nil, false
	}
	sheetID, err := uuid.Parse(raw)
	if err != nil {
		return uuid.Nil, false
	}
	return sheetID, true
}
