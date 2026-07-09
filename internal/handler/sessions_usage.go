package handler

import (
	"net/http"

	"github.com/usehivy/hivy/internal/billing"
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
		DebitedCredits int64
		UnbilledCost   float64
	}
	if err := h.db.WithContext(r.Context()).Raw(`
		SELECT
			COALESCE(SUM(credits_debited), 0) AS debited_credits,
			COALESCE(SUM(CASE WHEN billed_at IS NULL THEN GREATEST(cost, 0) ELSE 0 END), 0)::float8 AS unbilled_cost
		FROM generations
		WHERE session_id = ?
		  AND is_system = true`, session.ID).Scan(&row).Error; err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load session usage"})
		return
	}

	credits := row.DebitedCredits + billing.CostUSDToCredits(row.UnbilledCost)
	if credits < 0 {
		credits = 0
	}
	writeJSON(w, http.StatusOK, sessionUsageResponse{
		CostUSD: float64(credits) * billing.CreditUSDValue,
		Credits: credits,
	})
}
