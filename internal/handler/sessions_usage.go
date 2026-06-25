package handler

import (
	"net/http"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/runtimeevents"
)

type sessionUsageResponse struct {
	CostUSD float64 `json:"cost_usd"`
	Credits int64   `json:"credits"`
}

// GetUsage handles GET /v1/sessions/{id}/usage.
// @Summary Get session usage
// @Description Returns model usage cost and estimated credits for one visible session.
// @Tags sessions
// @Produce json
// @Param id path string true "Session ID"
// @Success 200 {object} sessionUsageResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/sessions/{id}/usage [get]
func (h *SessionHandler) GetUsage(w http.ResponseWriter, r *http.Request) {
	session, _, ok := h.authorizeSession(w, r, false)
	if !ok {
		return
	}

	var row struct {
		CostUSD float64
	}
	if err := h.db.WithContext(r.Context()).Raw(`
		SELECT
			COALESCE(SUM(
				CASE
					WHEN jsonb_typeof(payload->'usage'->'cost') = 'number'
						THEN GREATEST((payload->'usage'->>'cost')::numeric, 0)
					ELSE 0
				END
			), 0)::float8 AS cost_usd
		FROM session_events
		WHERE session_id = ?
		  AND event_type = ?`, session.ID, runtimeevents.EventModelUsage).Scan(&row).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load session usage"})
		return
	}

	if row.CostUSD < 0 {
		row.CostUSD = 0
	}
	writeJSON(w, http.StatusOK, sessionUsageResponse{
		CostUSD: row.CostUSD,
		Credits: billing.CostUSDToCredits(row.CostUSD),
	})
}
