package sandbox

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

type WarmPool struct {
	db       *gorm.DB
	provider Provider
	encKey   *crypto.SymmetricKey
	cfg      *config.Config
}

type ClaimedWarmSlot struct {
	ID            uuid.UUID
	ExternalID    string
	EndpointURL   string
	RuntimeSecret string
}

func NewWarmPool(db *gorm.DB, provider Provider, encKey *crypto.SymmetricKey, cfg *config.Config) *WarmPool {
	if _, ok := provider.(WarmSlotProvider); !ok {
		return nil
	}
	return &WarmPool{db: db, provider: provider, encKey: encKey, cfg: cfg}
}

func (p *WarmPool) DesiredCount(mode string) int {
	if p == nil || p.cfg == nil {
		return 0
	}
	switch mode {
	case model.SandboxWarmSlotModeAgent:
		return p.cfg.SandboxWarmPoolAgentSize
	default:
		return 0
	}
}

func (p *WarmPool) Claim(ctx context.Context, mode string, sandboxID uuid.UUID) (*ClaimedWarmSlot, error) {
	if p == nil {
		return nil, fmt.Errorf("warm pool is not configured")
	}
	image := p.runtimeImage(mode)
	if image == "" {
		return nil, fmt.Errorf("runtime image for warm %s sandbox is not configured", mode)
	}
	var slot model.SandboxWarmSlot
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE", Options: "SKIP LOCKED"}).
			Where("provider_id = ? AND mode = ? AND status = ? AND runtime_image = ?", p.provider.ID(), mode, model.SandboxWarmSlotStatusWarm, image).
			Order("created_at ASC").
			First(&slot).Error; err != nil {
			return err
		}
		return tx.Model(&slot).Updates(map[string]any{
			"status":             model.SandboxWarmSlotStatusClaiming,
			"claimed_sandbox_id": sandboxID,
			"error_message":      nil,
		}).Error
	})
	if err != nil {
		if err == gorm.ErrRecordNotFound {
			return nil, fmt.Errorf("no warm %s sandbox slots available", mode)
		}
		return nil, err
	}
	token, err := p.encKey.DecryptString(slot.EncryptedRuntimeSecret)
	if err != nil {
		_ = p.MarkError(context.WithoutCancel(ctx), slot.ID, fmt.Sprintf("decrypt runtime secret: %v", err))
		return nil, err
	}
	return &ClaimedWarmSlot{
		ID:            slot.ID,
		ExternalID:    slot.ExternalID,
		EndpointURL:   slot.EndpointURL,
		RuntimeSecret: token,
	}, nil
}

func (p *WarmPool) MarkClaimed(ctx context.Context, slotID uuid.UUID) error {
	return p.db.WithContext(ctx).Model(&model.SandboxWarmSlot{}).
		Where("id = ?", slotID).
		Update("status", model.SandboxWarmSlotStatusClaimed).Error
}

// MarkError flips a slot to error and deletes its provider resource. Nothing
// else processes error slots, so failing to delete leaks a live billing service
// forever; on a transient failure the slot parks in 'deleting' for the reaper.
func (p *WarmPool) MarkError(ctx context.Context, slotID uuid.UUID, message string) error {
	var slot model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).First(&slot, "id = ?", slotID).Error; err != nil {
		return err
	}
	if slot.ExternalID != "" {
		if err := p.provider.DeleteSandbox(ctx, slot.ExternalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
			logging.Capture(ctx, fmt.Errorf("mark warm slot %s error: delete provider resource %s: %w", slotID, slot.ExternalID, err))
			// Park in 'deleting' so the reaper retries rather than abandoning a live
			// resource in 'error'.
			_ = p.db.WithContext(ctx).Model(&model.SandboxWarmSlot{}).
				Where("id = ?", slotID).
				Updates(map[string]any{
					"status":        model.SandboxWarmSlotStatusDeleting,
					"error_message": message,
				}).Error
			return err
		}
	}
	return p.db.WithContext(ctx).Model(&model.SandboxWarmSlot{}).
		Where("id = ?", slotID).
		Updates(map[string]any{
			"status":        model.SandboxWarmSlotStatusError,
			"error_message": message,
		}).Error
}

// ReapStaleSlots releases provider resources for slots stranded in 'claiming' (a
// claim that crashed mid-flight) and retries deletion for 'deleting' slots,
// otherwise a dead claim leaks a live billing service no path ever deletes.
func (p *WarmPool) ReapStaleSlots(ctx context.Context) error {
	if p == nil {
		return nil
	}
	const claimingTTL = 10 * time.Minute
	cutoff := time.Now().Add(-claimingTTL)
	logger := logging.FromContext(ctx)

	var stale []model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).
		Where("provider_id = ? AND ((status = ? AND updated_at < ?) OR status = ?)",
			p.provider.ID(), model.SandboxWarmSlotStatusClaiming, cutoff, model.SandboxWarmSlotStatusDeleting).
		Find(&stale).Error; err != nil {
		return err
	}
	for i := range stale {
		slot := stale[i]
		if slot.ExternalID != "" {
			if err := p.provider.DeleteSandbox(ctx, slot.ExternalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
				logger.WarnContext(ctx, "reap stale warm slot: delete provider resource failed",
					"slot_id", slot.ID, "external_id", slot.ExternalID, "error", err)
				continue
			}
		}
		if err := p.db.WithContext(ctx).Model(&model.SandboxWarmSlot{}).
			Where("id = ?", slot.ID).
			Updates(map[string]any{
				"status":        model.SandboxWarmSlotStatusError,
				"error_message": "reaped stale warm slot",
			}).Error; err != nil {
			logger.WarnContext(ctx, "reap stale warm slot: mark error failed", "slot_id", slot.ID, "error", err)
			continue
		}
		logger.InfoContext(ctx, "reaped stale warm slot",
			"slot_id", slot.ID, "external_id", slot.ExternalID, "from_status", slot.Status)
	}
	return nil
}

func (p *WarmPool) SlotMode(ctx context.Context, slotID uuid.UUID) (string, error) {
	var slot model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).Select("mode").First(&slot, "id = ?", slotID).Error; err != nil {
		return "", err
	}
	return slot.Mode, nil
}
