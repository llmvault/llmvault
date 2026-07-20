package handler

import (
	"context"
	"errors"
	"net/http"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/orgtier"
)

// writeOrgTierError owns the HTTP mapping for capacity-tier limits. It returns
// false when err is not a tier-limit sentinel and the caller must handle it.
func writeOrgTierError(w http.ResponseWriter, err error) bool {
	switch {
	case errors.Is(err, orgtier.ErrConcurrentSessions):
		w.Header().Set("Retry-After", "15")
		writeJSON(w, http.StatusTooManyRequests, errorResponse{Error: "Your organization has reached its concurrent session limit. Stop a session or wait for one to sleep."})
		return true
	case errors.Is(err, orgtier.ErrSandboxSizeNotAllowed):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "This sandbox size is not available for your organization tier."})
		return true
	case errors.Is(err, orgtier.ErrKnowledgeStorageLimit):
		writeJSON(w, http.StatusUnprocessableEntity, errorResponse{Error: "Your organization has reached its knowledge storage limit."})
		return true
	default:
		return false
	}
}

func commitOrgTierWake(ctx context.Context, db *gorm.DB, reservation orgtier.WakeReservation) {
	if err := reservation.Commit(ctx, db); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "commit org tier wake reservation", "org_id", reservation.OrgID, "sandbox_id", reservation.SandboxID, "error", err)
	}
}

func rollbackOrgTierWake(ctx context.Context, db *gorm.DB, reservation orgtier.WakeReservation) {
	cleanupCtx := context.WithoutCancel(ctx)
	if err := reservation.Rollback(cleanupCtx, db); err != nil {
		logging.FromContext(cleanupCtx).ErrorContext(cleanupCtx, "rollback org tier wake reservation", "org_id", reservation.OrgID, "sandbox_id", reservation.SandboxID, "error", err)
	}
}
