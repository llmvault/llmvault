package sandbox

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
)

var activeWakeStatuses = []string{"creating", "starting", "running", "draining"}

// WakeReservation serializes attempts to wake one stopped sandbox. It is a
// lifecycle race guard, not a pricing or organization-capacity limit.
type WakeReservation struct {
	SandboxID uuid.UUID
	OrgID     uuid.UUID
	Reserved  bool
}

func ReserveWake(ctx context.Context, db *gorm.DB, orgID, sandboxID uuid.UUID) (WakeReservation, error) {
	reservation := WakeReservation{SandboxID: sandboxID, OrgID: orgID}
	if db == nil || sandboxID == uuid.Nil {
		return reservation, nil
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var sb model.Sandbox
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND org_id = ? AND agent_id IS NOT NULL", sandboxID, orgID).
			First(&sb).Error; err != nil {
			return fmt.Errorf("load session sandbox: %w", err)
		}
		for _, status := range activeWakeStatuses {
			if sb.Status == status {
				return nil
			}
		}
		if sb.Status != string(StatusStopped) {
			return nil
		}
		res := tx.Model(&model.Sandbox{}).
			Where("id = ? AND org_id = ? AND status = ?", sandboxID, orgID, StatusStopped).
			Updates(map[string]any{"status": StatusStarting, "updated_at": time.Now()})
		if res.Error != nil {
			return fmt.Errorf("reserve session wake: %w", res.Error)
		}
		reservation.Reserved = res.RowsAffected == 1
		return nil
	})
	return reservation, err
}

func (r WakeReservation) Commit(ctx context.Context, db *gorm.DB) error {
	if r.SandboxID == uuid.Nil || db == nil {
		return nil
	}
	return db.WithContext(ctx).Model(&model.Sandbox{}).
		Where("id = ? AND org_id = ? AND status = ?", r.SandboxID, r.OrgID, StatusStarting).
		Updates(map[string]any{"status": StatusRunning, "last_active_at": time.Now()}).Error
}

func (r WakeReservation) Rollback(ctx context.Context, db *gorm.DB) error {
	if !r.Reserved || db == nil {
		return nil
	}
	now := time.Now()
	return db.WithContext(ctx).Model(&model.Sandbox{}).
		Where("id = ? AND org_id = ? AND status = ?", r.SandboxID, r.OrgID, StatusStarting).
		Updates(map[string]any{"status": StatusStopped, "stopped_at": now}).Error
}
