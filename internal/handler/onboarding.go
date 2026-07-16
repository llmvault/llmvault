package handler

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/usehivy/hivy/internal/access"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/onboarding"
)

type advanceOnboardingRequest struct {
	Step string `json:"step"`
}

type onboardingResponse struct {
	Step string `json:"step"`
}

// AdvanceOnboarding handles PATCH /v1/orgs/current/onboarding.
// @Summary Advance onboarding
// @Description Advances the current organization through onboarding. Team creation and at least one active connection are required before their following steps.
// @Tags onboarding
// @Accept json
// @Produce json
// @Param body body advanceOnboardingRequest true "Next onboarding step"
// @Success 200 {object} onboardingResponse
// @Failure 400 {object} errorResponse
// @Failure 401 {object} errorResponse
// @Failure 403 {object} errorResponse
// @Failure 404 {object} errorResponse
// @Failure 500 {object} errorResponse
// @Security BearerAuth
// @Router /v1/orgs/current/onboarding [patch]
func (h *OrgHandler) AdvanceOnboarding(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	org, ok := middleware.OrgFromContext(ctx)
	if !ok || org == nil {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	actor, err := access.Resolve(ctx, h.db, org.ID, middleware.UserID(ctx))
	if err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "resolve onboarding actor", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	if !actor.IsOrgManager() {
		writeJSON(w, http.StatusForbidden, errorResponse{Error: "not permitted"})
		return
	}

	var req advanceOnboardingRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if err := onboarding.New(h.db).Advance(ctx, org.ID, req.Step); err != nil {
		if !errors.Is(err, onboarding.ErrInvalidTransition) &&
			!errors.Is(err, onboarding.ErrConnectionRequired) &&
			!errors.Is(err, onboarding.ErrNotFound) {
			logging.FromContext(ctx).ErrorContext(ctx, "advance onboarding", "error", err, "org_id", org.ID)
		}
		writeOnboardingError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, onboardingResponse(req))
}

func writeOnboardingError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, onboarding.ErrConnectionRequired):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "add at least one connection before continuing"})
	case errors.Is(err, onboarding.ErrInvalidTransition):
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "onboarding steps must be completed in order"})
	case errors.Is(err, onboarding.ErrNotFound):
		writeJSON(w, http.StatusNotFound, errorResponse{Error: "organization not found"})
	default:
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to advance onboarding"})
	}
}
