package handler

import (
	"encoding/base64"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type activityPaginationCursor struct {
	LastActivity time.Time
	CreatedAt    time.Time
	ID           uuid.UUID
}

func parseSessionListPagination(r *http.Request, sort string) (int, *paginationCursor, *activityPaginationCursor, error) {
	limit, err := parsePaginationLimit(r)
	if err != nil {
		return 0, nil, nil, err
	}
	raw := r.URL.Query().Get("cursor")
	if raw == "" || raw == "0" {
		return limit, nil, nil, nil
	}
	if sort == sessionSortActivity {
		cursor, err := decodeActivityCursor(raw)
		if err != nil {
			return 0, nil, nil, fmt.Errorf("invalid cursor")
		}
		return limit, nil, cursor, nil
	}
	cursor, err := decodeCursor(raw)
	if err != nil {
		return 0, nil, nil, fmt.Errorf("invalid cursor")
	}
	return limit, cursor, nil, nil
}

func encodeActivityCursor(lastActivity, createdAt time.Time, id uuid.UUID) string {
	s := lastActivity.Format(time.RFC3339Nano) + "|" + createdAt.Format(time.RFC3339Nano) + "|" + id.String()
	return base64.URLEncoding.EncodeToString([]byte(s))
}

func decodeActivityCursor(s string) (*activityPaginationCursor, error) {
	b, err := base64.URLEncoding.DecodeString(s)
	if err != nil {
		return nil, err
	}
	first := splitOnce(string(b), '|')
	if len(first) != 2 {
		return nil, fmt.Errorf("malformed cursor")
	}
	second := splitOnce(first[1], '|')
	if len(second) != 2 {
		return nil, fmt.Errorf("malformed cursor")
	}
	lastActivity, err := time.Parse(time.RFC3339Nano, first[0])
	if err != nil {
		return nil, err
	}
	createdAt, err := time.Parse(time.RFC3339Nano, second[0])
	if err != nil {
		return nil, err
	}
	id, err := uuid.Parse(second[1])
	if err != nil {
		return nil, err
	}
	return &activityPaginationCursor{LastActivity: lastActivity, CreatedAt: createdAt, ID: id}, nil
}

func applySessionActivityPagination(q *gorm.DB, cursor *activityPaginationCursor, limit int) *gorm.DB {
	q = q.Joins("LEFT JOIN LATERAL (SELECT max(event_at) AS last_event FROM session_events se WHERE se.session_id = sessions.id) AS session_activity ON TRUE")
	if cursor != nil {
		q = q.Where(
			"(COALESCE(session_activity.last_event, sessions.updated_at), sessions.created_at, sessions.id) < (?, ?, ?)",
			cursor.LastActivity,
			cursor.CreatedAt,
			cursor.ID,
		)
	}
	return q.Order("COALESCE(session_activity.last_event, sessions.updated_at) DESC, sessions.created_at DESC, sessions.id DESC").Limit(limit + 1)
}

func sessionResponsesFromStats(sessions []model.Session, stats map[uuid.UUID]sessionStats) []sessionResponse {
	out := make([]sessionResponse, len(sessions))
	for i, session := range sessions {
		stat := stats[session.ID]
		out[i] = sessionToResponse(session, stat.ParticipantCount, stat.EventCount, stat.LastEvent)
	}
	return out
}

func sessionActivityTime(session model.Session, stats sessionStats) time.Time {
	if stats.LastEvent != nil && !stats.LastEvent.IsZero() {
		return *stats.LastEvent
	}
	return session.UpdatedAt
}
