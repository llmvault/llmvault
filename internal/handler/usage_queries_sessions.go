package handler

import (
	"fmt"
	"time"

	"github.com/google/uuid"
)

func (h *UsageHandler) querySessions(orgID uuid.UUID) ([]sessionSummary, error) {
	var rows []struct {
		ID         uuid.UUID
		Name       string
		Status     string
		Source     string
		EventCount int64
		CreatedAt  time.Time
		EndedAt    *time.Time
	}
	if err := h.db.Raw(`
		SELECT s.id, s.name, s.status, s.source,
			COALESCE(COUNT(se.id), 0) AS event_count,
			s.created_at, s.ended_at
		FROM sessions s
		LEFT JOIN session_events se ON se.session_id = s.id
		WHERE s.org_id = ? AND s.status <> 'archived'
		GROUP BY s.id
		ORDER BY s.created_at DESC
		LIMIT 50`, orgID).Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("sessions: %w", err)
	}
	result := make([]sessionSummary, 0, len(rows))
	for _, row := range rows {
		s := sessionSummary{
			ID:         row.ID.String(),
			Name:       row.Name,
			Status:     row.Status,
			Source:     row.Source,
			EventCount: row.EventCount,
			CreatedAt:  row.CreatedAt.UTC().Format(time.RFC3339),
		}
		if row.EndedAt != nil {
			t := row.EndedAt.UTC().Format(time.RFC3339)
			s.EndedAt = &t
		}
		result = append(result, s)
	}
	return result, nil
}
