package runtimestream

import (
	"encoding/json"
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

const (
	DefaultShardCount           = 64
	DefaultStreamMaxLen         = int64(100000)
	DefaultSessionCheckpointTTL = 7 * 24 * time.Hour

	DurabilityPreview = "preview"
	DurabilityDurable = "durable"

	LiveKindRuntime   = "runtime"
	LiveKindCommitted = "committed"
	LiveKindNotice    = "notice"

	NoticeTypeArtifactSynced = "artifact.synced"
	NoticeTypeUsageUpdated   = "usage.updated"

	ProjectorGroup = "runtime-db-projector"
)

type Event struct {
	SessionID   string         `json:"session_id"`
	SandboxID   string         `json:"sandbox_id,omitempty"`
	AgentID     string         `json:"agent_id,omitempty"`
	OrgID       string         `json:"org_id,omitempty"`
	TurnID      string         `json:"turn_id,omitempty"`
	StreamID    string         `json:"stream_id,omitempty"`
	RuntimeSeq  int64          `json:"runtime_seq"`
	EventID     string         `json:"event_id,omitempty"`
	EventType   string         `json:"event_type"`
	Durability  string         `json:"durability"`
	SpanID      string         `json:"span_id,omitempty"`
	Source      string         `json:"source,omitempty"`
	ActorUserID string         `json:"actor_user_id,omitempty"`
	Payload     map[string]any `json:"payload,omitempty"`
	OccurredAt  time.Time      `json:"occurred_at"`
}

type LiveMessage struct {
	Kind       string            `json:"kind"`
	SessionID  string            `json:"session_id"`
	RuntimeSeq int64             `json:"runtime_seq,omitempty"`
	Event      *Event            `json:"event,omitempty"`
	Committed  *SessionEventView `json:"committed,omitempty"`
	Notice     *Notice           `json:"notice,omitempty"`
	Published  time.Time         `json:"published_at"`
}

type Notice struct {
	Type        string          `json:"type"`
	OrgID       uuid.UUID       `json:"org_id"`
	SessionID   *uuid.UUID      `json:"session_id,omitempty"`
	Data        json.RawMessage `json:"data"`
	PublishedAt time.Time       `json:"published_at"`
}

type SessionEventView struct {
	ID             string         `json:"id"`
	SessionID      string         `json:"session_id"`
	AgentID        string         `json:"agent_id"`
	SandboxID      *string        `json:"sandbox_id,omitempty"`
	EventID        string         `json:"event_id,omitempty"`
	EventType      string         `json:"event_type"`
	ActorUserID    *string        `json:"actor_user_id,omitempty"`
	Source         string         `json:"source"`
	SequenceNumber int64          `json:"sequence_number"`
	RuntimeSeq     *int64         `json:"runtime_seq,omitempty"`
	RuntimeEventID string         `json:"runtime_event_id,omitempty"`
	TurnID         string         `json:"turn_id,omitempty"`
	SpanID         string         `json:"span_id,omitempty"`
	Durability     string         `json:"durability,omitempty"`
	Payload        map[string]any `json:"payload"`
	EventAt        string         `json:"event_at"`
}

func (e *Event) Normalize() {
	if e == nil {
		return
	}
	e.SessionID = strings.TrimSpace(e.SessionID)
	e.SandboxID = strings.TrimSpace(e.SandboxID)
	e.AgentID = strings.TrimSpace(e.AgentID)
	e.OrgID = strings.TrimSpace(e.OrgID)
	e.TurnID = strings.TrimSpace(e.TurnID)
	e.StreamID = strings.TrimSpace(e.StreamID)
	e.EventID = strings.TrimSpace(e.EventID)
	e.EventType = strings.TrimSpace(e.EventType)
	e.Durability = strings.TrimSpace(strings.ToLower(e.Durability))
	e.SpanID = strings.TrimSpace(e.SpanID)
	e.Source = strings.TrimSpace(e.Source)
	e.ActorUserID = strings.TrimSpace(e.ActorUserID)
	if e.Durability == "" {
		e.Durability = DurabilityPreview
	}
	if e.Payload == nil {
		e.Payload = map[string]any{}
	}
	if e.OccurredAt.IsZero() {
		e.OccurredAt = time.Now().UTC()
	} else {
		e.OccurredAt = e.OccurredAt.UTC()
	}
}

func (e Event) Validate() error {
	if strings.TrimSpace(e.SessionID) == "" {
		return fmt.Errorf("session_id is required")
	}
	if _, err := uuid.Parse(e.SessionID); err != nil {
		return fmt.Errorf("session_id must be a uuid: %w", err)
	}
	if e.RuntimeSeq <= 0 {
		return fmt.Errorf("runtime_seq must be positive")
	}
	if strings.TrimSpace(e.EventType) == "" {
		return fmt.Errorf("event_type is required")
	}
	switch strings.TrimSpace(strings.ToLower(e.Durability)) {
	case DurabilityPreview, DurabilityDurable:
		return nil
	default:
		return fmt.Errorf("durability must be %q or %q", DurabilityPreview, DurabilityDurable)
	}
}

func (e Event) Marshal() ([]byte, error) {
	normalized := e
	normalized.Normalize()
	if err := normalized.Validate(); err != nil {
		return nil, err
	}
	return json.Marshal(normalized)
}

func ParseEventJSON(raw []byte) (Event, error) {
	var event Event
	if err := json.Unmarshal(raw, &event); err != nil {
		return Event{}, err
	}
	event.Normalize()
	if err := event.Validate(); err != nil {
		return Event{}, err
	}
	return event, nil
}

func StreamKey(shard int) string {
	return "runtime_events:" + strconv.Itoa(shard)
}

func LiveChannel(sessionID string) string {
	return "runtime_session:{" + strings.TrimSpace(sessionID) + "}:live"
}

func LastSeqKey(sessionID string) string {
	return "runtime_session:{" + strings.TrimSpace(sessionID) + "}:last_seq"
}

func EventIndexKey(sessionID string) string {
	return "runtime_session:{" + strings.TrimSpace(sessionID) + "}:event_index"
}

func ProjectedSeqKey(sessionID string) string {
	return "runtime_session:{" + strings.TrimSpace(sessionID) + "}:projected_seq"
}

func ShardForSession(sessionID string, shardCount int) int {
	if shardCount <= 0 {
		shardCount = DefaultShardCount
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(strings.TrimSpace(sessionID)))
	shards := uint32(shardCount) // #nosec G115 -- shardCount is positive after the guard above.
	return int(h.Sum32() % shards)
}

func EventFromStreamValues(values map[string]any) (Event, error) {
	raw, ok := values["event"]
	if !ok {
		return Event{}, fmt.Errorf("stream message missing event field")
	}
	switch v := raw.(type) {
	case string:
		return ParseEventJSON([]byte(v))
	case []byte:
		return ParseEventJSON(v)
	default:
		return Event{}, fmt.Errorf("stream event field has unsupported type %T", raw)
	}
}

func SessionEventToView(event model.SessionEvent) SessionEventView {
	payload := map[string]any{}
	for key, value := range event.Payload {
		payload[key] = value
	}
	return SessionEventView{
		ID:             event.ID.String(),
		SessionID:      event.SessionID.String(),
		AgentID:        event.AgentID.String(),
		SandboxID:      formatUUIDPtr(event.SandboxID),
		EventID:        event.EventID,
		EventType:      event.EventType,
		ActorUserID:    formatUUIDPtr(event.ActorUserID),
		Source:         event.Source,
		SequenceNumber: event.SequenceNumber,
		RuntimeSeq:     event.RuntimeSeq,
		RuntimeEventID: event.RuntimeEventID,
		TurnID:         event.TurnID,
		SpanID:         event.SpanID,
		Durability:     event.Durability,
		Payload:        payload,
		EventAt:        event.EventAt.UTC().Format(time.RFC3339Nano),
	}
}

func formatUUIDPtr(id *uuid.UUID) *string {
	if id == nil || *id == uuid.Nil {
		return nil
	}
	value := id.String()
	return &value
}
