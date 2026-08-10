package handler

import (
	"net/http"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/logging"
)

type sessionUsageResponse struct {
	CostUSD            float64 `json:"cost_usd"`
	Credits            float64 `json:"credits"`
	ModelCostUSD       float64 `json:"model_cost_usd"`
	ModelCredits       int64   `json:"model_credits"`
	SandboxCostUSD     float64 `json:"sandbox_cost_usd"`
	SandboxCredits     float64 `json:"sandbox_credits"`
	SandboxVCPUSeconds int64   `json:"sandbox_vcpu_seconds"`
}

// GetUsage handles GET /v1/sessions/{id}/usage.
// @Summary Get session usage
// @Description Returns model and sandbox usage for one visible session.
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

	modelCredits := row.DebitedCredits + billing.CostUSDToCredits(row.UnbilledCost)
	if modelCredits < 0 {
		modelCredits = 0
	}
	var sandboxRow struct {
		WeightedMilliseconds int64
		VCPUMilliseconds     int64
	}
	if err := h.db.WithContext(r.Context()).Raw(`
		SELECT COALESCE(SUM(active_milliseconds * sandbox_vcpu * credits_per_vcpu_minute), 0) AS weighted_milliseconds,
		       COALESCE(SUM(active_milliseconds * sandbox_vcpu), 0) AS v_cpu_milliseconds
		FROM sandbox_turn_usage
		WHERE session_id = ?`, session.ID).Scan(&sandboxRow).Error; err != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "load session sandbox usage", "session_id", session.ID, "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to load session sandbox usage"})
		return
	}
	sandboxCredits := float64(sandboxRow.WeightedMilliseconds) / 60_000
	modelCostUSD := float64(modelCredits) * billing.CreditUSDValue
	sandboxCostUSD := sandboxCredits * billing.CreditUSDValue
	writeJSON(w, http.StatusOK, sessionUsageResponse{
		CostUSD: modelCostUSD + sandboxCostUSD, Credits: float64(modelCredits) + sandboxCredits,
		ModelCostUSD: modelCostUSD, ModelCredits: modelCredits,
		SandboxCostUSD: sandboxCostUSD, SandboxCredits: sandboxCredits,
		SandboxVCPUSeconds: sandboxRow.VCPUMilliseconds / 1_000,
	})
}
