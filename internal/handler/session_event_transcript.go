package handler

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/usehivy/hivy/internal/model"
)

const (
	sessionEventViewRaw        = "raw"
	sessionEventViewTranscript = "transcript"
	transcriptScanChunkSize    = 1000
)

var transcriptTelemetryEventTypes = []string{
	"model_usage",
	"model_request_started",
	"model_request_completed",
}

func parseSessionEventView(r *http.Request) (string, error) {
	view := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("view")))
	if view == "" {
		return sessionEventViewRaw, nil
	}
	switch view {
	case sessionEventViewRaw, sessionEventViewTranscript:
		return view, nil
	default:
		return "", fmt.Errorf("invalid event view")
	}
}

func (h *SessionHandler) listTranscriptEvents(
	ctx context.Context,
	session model.Session,
	cursor *sessionEventPaginationCursor,
	limit int,
) ([]model.SessionEvent, bool, error) {
	var completedTurnIDs []string
	if err := h.db.WithContext(ctx).
		Model(&model.SessionEvent{}).
		Where("session_id = ? AND org_id = ? AND event_type = ? AND turn_id <> ''", session.ID, session.OrgID, "final").
		Distinct("turn_id").
		Pluck("turn_id", &completedTurnIDs).Error; err != nil {
		return nil, false, err
	}

	events := make([]model.SessionEvent, 0, limit+1)
	var pending *transcriptDeltaGroup
	scanCursor := cursor
	hasMore := false

	flushPending := func() {
		if pending == nil {
			return
		}
		events = append(events, pending.event())
		pending = nil
	}

	for {
		query := h.db.WithContext(ctx).
			Where("session_id = ? AND org_id = ?", session.ID, session.OrgID).
			Where("event_type NOT IN ?", transcriptTelemetryEventTypes)
		if len(completedTurnIDs) > 0 {
			query = query.Where("NOT (event_type = ? AND turn_id IN ?)", "token", completedTurnIDs)
		}
		query = applySessionEventPagination(query, scanCursor, transcriptScanChunkSize-1)
		var chunk []model.SessionEvent
		if err := query.Find(&chunk).Error; err != nil {
			return nil, false, err
		}
		if len(chunk) == 0 {
			flushPending()
			break
		}

		for _, event := range chunk {
			if key, text, ok := transcriptDelta(event); ok {
				if pending != nil && pending.key == key {
					pending.prepend(event, text)
					continue
				}
				flushPending()
				pending = newTranscriptDeltaGroup(key, event, text)
			} else {
				flushPending()
				events = append(events, event)
			}
			if len(events) > limit {
				hasMore = true
				break
			}
		}
		if hasMore {
			break
		}
		if len(events) >= limit && pending != nil {
			hasMore = true
			break
		}
		if len(chunk) < transcriptScanChunkSize {
			flushPending()
			break
		}
		last := chunk[len(chunk)-1]
		scanCursor = &sessionEventPaginationCursor{SequenceNumber: last.SequenceNumber, ID: last.ID}
	}

	if len(events) > limit {
		hasMore = true
		events = events[:limit]
	}
	return events, hasMore, nil
}

type transcriptDeltaGroup struct {
	key        string
	newest     model.SessionEvent
	oldest     model.SessionEvent
	text       string
	deltaCount int
}

func newTranscriptDeltaGroup(key string, event model.SessionEvent, text string) *transcriptDeltaGroup {
	return &transcriptDeltaGroup{
		key:        key,
		newest:     event,
		oldest:     event,
		text:       text,
		deltaCount: 1,
	}
}

func (group *transcriptDeltaGroup) prepend(event model.SessionEvent, text string) {
	group.oldest = event
	group.text = text + group.text
	group.deltaCount++
}

func (group *transcriptDeltaGroup) event() model.SessionEvent {
	event := group.newest
	event.ID = group.oldest.ID
	event.EventID = group.oldest.EventID
	event.SequenceNumber = group.oldest.SequenceNumber
	event.RuntimeSeq = group.oldest.RuntimeSeq
	event.RuntimeEventID = group.oldest.RuntimeEventID
	event.EventAt = group.oldest.EventAt
	event.CreatedAt = group.oldest.CreatedAt
	event.Payload = cloneSessionEventPayload(group.newest.Payload)
	event.Payload["text"] = group.text
	event.Payload["coalesced"] = true
	event.Payload["delta_count"] = group.deltaCount
	event.Payload["sequence_start"] = group.oldest.SequenceNumber
	event.Payload["sequence_end"] = group.newest.SequenceNumber
	return event
}

func transcriptDelta(event model.SessionEvent) (string, string, bool) {
	if event.EventType != "thinking" && event.EventType != "token" {
		return "", "", false
	}
	text, _ := event.Payload["text"].(string)
	if text == "" {
		return "", "", false
	}
	scope, _ := event.Payload["scope"].(string)
	subagent, _ := json.Marshal(event.Payload["subagent"])
	key := strings.Join([]string{event.EventType, event.TurnID, scope, string(subagent)}, "|")
	return key, text, true
}

func cloneSessionEventPayload(payload model.JSON) model.JSON {
	cloned := make(model.JSON, len(payload)+4)
	for key, value := range payload {
		cloned[key] = value
	}
	return cloned
}
