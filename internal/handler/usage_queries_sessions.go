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
		SELECT es.id, es.name, es.status, es.source,
			COALESCE(COUNT(ese.id), 0) AS event_count,
			es.created_at, es.ended_at
		FROM employee_sessions es
		LEFT JOIN employee_session_events ese ON ese.employee_session_id = es.id
		WHERE es.org_id = ?
		GROUP BY es.id
		ORDER BY es.created_at DESC
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
