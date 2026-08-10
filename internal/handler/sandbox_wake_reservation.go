package handler

import (
	"context"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/sandbox"
)

func commitSandboxWake(ctx context.Context, db *gorm.DB, reservation sandbox.WakeReservation) {
	if err := reservation.Commit(ctx, db); err != nil {
		logging.FromContext(ctx).ErrorContext(ctx, "commit sandbox wake reservation", "org_id", reservation.OrgID, "sandbox_id", reservation.SandboxID, "error", err)
	}
}

func rollbackSandboxWake(ctx context.Context, db *gorm.DB, reservation sandbox.WakeReservation) {
	cleanupCtx := context.WithoutCancel(ctx)
	if err := reservation.Rollback(cleanupCtx, db); err != nil {
		logging.FromContext(cleanupCtx).ErrorContext(cleanupCtx, "rollback sandbox wake reservation", "org_id", reservation.OrgID, "sandbox_id", reservation.SandboxID, "error", err)
	}
}
