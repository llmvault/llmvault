package orgtier

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const sessionCreateReservationLifetime = 10 * time.Minute

var activeSandboxStatuses = []string{"creating", "starting", "running", "draining"}

// WithSessionCreate atomically claims a short-lived org capacity reservation,
// then releases the database transaction before the external sandbox create.
func WithSessionCreate(ctx context.Context, db *gorm.DB, orgID uuid.UUID, sandboxSize string, create func() error) error {
	if db == nil || create == nil {
		return fmt.Errorf("org session capacity: database and create callback are required")
	}
	reservation := model.OrgSessionCapacityReservation{
		ID: uuid.New(), OrgID: orgID, ExpiresAt: time.Now().Add(sessionCreateReservationLifetime),
	}
	if err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		org, err := lockOrg(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if err := ValidateSandboxSize(org.CapacityTier, sandboxSize); err != nil {
			return err
		}
		active, err := activeAgentSandboxCount(ctx, tx, orgID)
		if err != nil {
			return err
		}
		pending, err := pendingSessionReservationCount(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if active+pending >= int64(LimitsForTier(org.CapacityTier).ConcurrentSessions) {
			return ErrConcurrentSessions
		}
		if err := tx.WithContext(ctx).Create(&reservation).Error; err != nil {
			return fmt.Errorf("create org session reservation: %w", err)
		}
		return nil
	}); err != nil {
		return err
	}

	createErr := create()
	cleanupCtx := context.WithoutCancel(ctx)
	cleanupErr := db.WithContext(cleanupCtx).
		Where("id = ? AND org_id = ?", reservation.ID, orgID).
		Delete(&model.OrgSessionCapacityReservation{}).Error
	if cleanupErr != nil {
		logging.FromContext(cleanupCtx).ErrorContext(cleanupCtx, "release org session capacity reservation", "org_id", orgID, "reservation_id", reservation.ID, "error", cleanupErr)
	}
	if createErr != nil {
		return createErr
	}
	return nil
}

type WakeReservation struct {
	SandboxID uuid.UUID
	OrgID     uuid.UUID
	Reserved  bool
}

// ReserveSessionWake atomically turns a stopped sandbox into a starting row.
func ReserveSessionWake(ctx context.Context, db *gorm.DB, orgID, sandboxID uuid.UUID) (WakeReservation, error) {
	reservation := WakeReservation{SandboxID: sandboxID, OrgID: orgID}
	if db == nil || sandboxID == uuid.Nil {
		return reservation, nil
	}
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		org, err := lockOrg(ctx, tx, orgID)
		if err != nil {
			return err
		}
		var sb model.Sandbox
		if err := tx.WithContext(ctx).
			Where("id = ? AND org_id = ? AND agent_id IS NOT NULL", sandboxID, orgID).
			First(&sb).Error; err != nil {
			return fmt.Errorf("load org session sandbox: %w", err)
		}
		if contains(activeSandboxStatuses, sb.Status) {
			return nil
		}
		active, err := activeAgentSandboxCount(ctx, tx, orgID)
		if err != nil {
			return err
		}
		pending, err := pendingSessionReservationCount(ctx, tx, orgID)
		if err != nil {
			return err
		}
		if active+pending >= int64(LimitsForTier(org.CapacityTier).ConcurrentSessions) {
			return ErrConcurrentSessions
		}
		res := tx.WithContext(ctx).Model(&model.Sandbox{}).
			Where("id = ? AND org_id = ? AND status = ?", sandboxID, orgID, "stopped").
			Updates(map[string]any{"status": "starting", "updated_at": time.Now()})
		if res.Error != nil {
			return fmt.Errorf("reserve org session wake: %w", res.Error)
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
		Where("id = ? AND org_id = ? AND status = ?", r.SandboxID, r.OrgID, "starting").
		Updates(map[string]any{"status": "running", "last_active_at": time.Now()}).Error
}

func (r WakeReservation) Rollback(ctx context.Context, db *gorm.DB) error {
	if !r.Reserved || db == nil {
		return nil
	}
	now := time.Now()
	return db.WithContext(ctx).Model(&model.Sandbox{}).
		Where("id = ? AND org_id = ? AND status = ?", r.SandboxID, r.OrgID, "starting").
		Updates(map[string]any{"status": "stopped", "stopped_at": now}).Error
}

func activeAgentSandboxCount(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (int64, error) {
	var count int64
	if err := db.WithContext(ctx).Model(&model.Sandbox{}).
		Where("org_id = ? AND agent_id IS NOT NULL AND status IN ?", orgID, activeSandboxStatuses).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count active org sessions: %w", err)
	}
	return count, nil
}

func pendingSessionReservationCount(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (int64, error) {
	now := time.Now()
	if err := db.WithContext(ctx).
		Where("org_id = ? AND expires_at <= ?", orgID, now).
		Delete(&model.OrgSessionCapacityReservation{}).Error; err != nil {
		return 0, fmt.Errorf("delete expired org session reservations: %w", err)
	}
	var count int64
	if err := db.WithContext(ctx).Model(&model.OrgSessionCapacityReservation{}).
		Where("org_id = ? AND expires_at > ?", orgID, now).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("count org session reservations: %w", err)
	}
	return count, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
