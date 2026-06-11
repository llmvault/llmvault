package sandbox

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"strings"
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
	case model.SandboxWarmSlotModeEmployee:
		return p.cfg.SandboxWarmPoolEmployeeSize
	case model.SandboxWarmSlotModeSpecialist:
		return p.cfg.SandboxWarmPoolSpecialistSize
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

// MarkError flips a slot to the terminal error state and deletes its provider
// resource so the paid compute is released. Nothing else in the system ever
// processes error slots, so failing to delete here leaks a live billing service
// forever. The provider delete runs first; on a transient delete failure the
// slot is parked in 'deleting' and the periodic reaper retries.
func (p *WarmPool) MarkError(ctx context.Context, slotID uuid.UUID, message string) error {
	var slot model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).First(&slot, "id = ?", slotID).Error; err != nil {
		return err
	}
	if slot.ExternalID != "" {
		if err := p.provider.DeleteSandbox(ctx, slot.ExternalID); err != nil && !errors.Is(err, ErrSandboxNotFound) {
			logging.Capture(ctx, fmt.Errorf("mark warm slot %s error: delete provider resource %s: %w", slotID, slot.ExternalID, err))
			// Leave a breadcrumb in 'deleting' so the reaper retries the delete
			// rather than abandoning the slot in 'error' with a live resource.
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

// ReapStaleSlots releases provider resources for warm slots stranded in
// 'claiming' (a claim that crashed mid-flight before MarkClaimed/MarkError) and
// retries provider deletion for slots left in 'deleting'. Without this a claim
// that dies between Claim and the orchestrator finishing leaves a live billing
// service that no path ever deletes.
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

func (p *WarmPool) Reconcile(ctx context.Context, mode string, onCreated func(context.Context, uuid.UUID) error) ([]uuid.UUID, error) {
	if p == nil {
		return nil, nil
	}
	desired := p.DesiredCount(mode)
	if desired <= 0 {
		return nil, nil
	}
	image := p.runtimeImage(mode)
	if image == "" {
		return nil, fmt.Errorf("runtime image for warm %s sandbox is not configured", mode)
	}

	// Serialise reconciles for this (provider, mode) across pods. Without this the
	// count-then-provision below races: two reconciles each read 'available < desired'
	// and both provision, permanently over-provisioning paid services. A
	// transaction-scoped advisory lock blocks concurrent reconciles for the same
	// (provider, mode) while letting unrelated modes/providers proceed.
	var created []uuid.UUID
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", warmPoolReconcileLockKey(p.provider.ID(), mode)).Error; err != nil {
			return fmt.Errorf("acquire warm pool reconcile lock: %w", err)
		}
		var rerr error
		created, rerr = p.reconcileLocked(ctx, tx, mode, image, desired, onCreated)
		return rerr
	})
	return created, err
}

func (p *WarmPool) reconcileLocked(ctx context.Context, tx *gorm.DB, mode, image string, desired int, onCreated func(context.Context, uuid.UUID) error) ([]uuid.UUID, error) {
	logger := logging.FromContext(ctx)
	if err := p.deleteStaleAvailableSlots(ctx, mode, image); err != nil {
		return nil, err
	}

	var availableSlots []model.SandboxWarmSlot
	if err := tx.WithContext(ctx).
		Where("provider_id = ? AND mode = ? AND runtime_image = ? AND status IN ?", p.provider.ID(), mode, image, []string{
			model.SandboxWarmSlotStatusWarm,
			model.SandboxWarmSlotStatusWarming,
		}).
		Order("created_at ASC").
		Find(&availableSlots).Error; err != nil {
		return nil, err
	}
	available := int64(len(availableSlots))
	logger.InfoContext(ctx, "sandbox warm pool reconcile",
		"provider", p.provider.ID(), "mode", mode, "desired", desired, "available", available,
		"runtime_image", image)

	// Scale down: trim surplus warm slots beyond desired so a lowered pool size
	// (or a transient over-provision) releases the extra paid services instead of
	// leaving them running forever. Prefer trimming 'warm' (idle) slots over
	// 'warming' ones, oldest first.
	if int(available) > desired {
		surplus := int(available) - desired
		if err := p.trimSurplusSlots(ctx, availableSlots, surplus); err != nil {
			return nil, err
		}
		return nil, nil
	}

	created := make([]uuid.UUID, 0, desired-int(available))
	for i := available; i < int64(desired); i++ {
		slotID, err := p.provision(ctx, mode)
		if err != nil {
			return created, err
		}
		created = append(created, slotID)
		if onCreated != nil {
			if err := onCreated(ctx, slotID); err != nil {
				return created, err
			}
		}
	}
	var warming []model.SandboxWarmSlot
	if err := tx.WithContext(ctx).
		Where("provider_id = ? AND mode = ? AND runtime_image = ? AND status = ?", p.provider.ID(), mode, image, model.SandboxWarmSlotStatusWarming).
		Find(&warming).Error; err != nil {
		return created, err
	}
	for _, slot := range warming {
		if !containsUUID(created, slot.ID) {
			created = append(created, slot.ID)
			if onCreated != nil {
				if err := onCreated(ctx, slot.ID); err != nil {
					return created, err
				}
			}
		}
	}
	return created, nil
}

// trimSurplusSlots releases the `surplus` newest idle warm slots, preferring
// fully 'warm' slots over still-'warming' ones. Deletion runs through MarkError
// so the provider resource is released (P0-19) rather than just flipping the row.
func (p *WarmPool) trimSurplusSlots(ctx context.Context, slots []model.SandboxWarmSlot, surplus int) error {
	if surplus <= 0 {
		return nil
	}
	// Trim newest-first so the longest-lived (most likely already healthy) slots
	// survive; within that, warm before warming.
	ordered := make([]model.SandboxWarmSlot, 0, len(slots))
	for _, s := range slots {
		if s.Status == model.SandboxWarmSlotStatusWarm {
			ordered = append(ordered, s)
		}
	}
	for _, s := range slots {
		if s.Status == model.SandboxWarmSlotStatusWarming {
			ordered = append(ordered, s)
		}
	}
	logger := logging.FromContext(ctx)
	for i := len(ordered) - 1; i >= 0 && surplus > 0; i-- {
		slot := ordered[i]
		logger.InfoContext(ctx, "trimming surplus sandbox warm slot",
			"provider", p.provider.ID(), "mode", slot.Mode, "slot_id", slot.ID,
			"external_id", slot.ExternalID, "status", slot.Status)
		if err := p.MarkError(ctx, slot.ID, "trimmed surplus warm slot"); err != nil {
			return fmt.Errorf("trim surplus warm slot %s: %w", slot.ID, err)
		}
		surplus--
	}
	return nil
}

// warmPoolReconcileLockKey derives a stable bigint advisory-lock key for a
// (provider, mode) pair. Collisions only ever serialise unrelated reconciles,
// which is harmless.
func warmPoolReconcileLockKey(providerID, mode string) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("warm-pool-reconcile:"))
	_, _ = h.Write([]byte(providerID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(mode))
	return int64(h.Sum64()) // #nosec G115 -- hash truncation; sign bit is part of the hash distribution
}

func (p *WarmPool) provision(ctx context.Context, mode string) (uuid.UUID, error) {
	provider, ok := p.provider.(WarmSlotProvider)
	if !ok {
		return uuid.Nil, fmt.Errorf("provider %s does not support warm slots", p.provider.ID())
	}
	runtimeSecret, err := generateRandomHex(32)
	if err != nil {
		return uuid.Nil, fmt.Errorf("generate runtime secret: %w", err)
	}
	encrypted, err := p.encKey.EncryptString(runtimeSecret)
	if err != nil {
		return uuid.Nil, fmt.Errorf("encrypt runtime secret: %w", err)
	}
	image := p.runtimeImage(mode)
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "sandbox warm slot provisioning",
		"provider", p.provider.ID(), "mode", mode, "image", image, "port", p.cfg.RailwayRuntimePort)
	info, err := provider.CreateWarmSlot(ctx, WarmSlotCreateOpts{
		Name:          p.slotName(mode),
		Mode:          mode,
		RuntimeImage:  image,
		RuntimePort:   p.cfg.RailwayRuntimePort,
		RuntimeSecret: runtimeSecret,
		EnvVars:       p.warmSlotEnvVars(),
		Labels: map[string]string{
			"mode":     mode,
			"provider": p.provider.ID(),
		},
	})
	if err != nil {
		return uuid.Nil, err
	}
	logger.InfoContext(ctx, "sandbox warm slot provider resource created",
		"provider", p.provider.ID(), "mode", mode, "external_id", info.ExternalID,
		"endpoint_url", info.EndpointURL, "port", info.RuntimePort)
	slot := model.SandboxWarmSlot{
		ProviderID:             p.provider.ID(),
		Mode:                   mode,
		Status:                 model.SandboxWarmSlotStatusWarming,
		ExternalID:             info.ExternalID,
		EndpointURL:            info.EndpointURL,
		RuntimeImage:           image,
		RuntimePort:            info.RuntimePort,
		Region:                 p.cfg.RailwayRegion,
		EncryptedRuntimeSecret: encrypted,
	}
	if err := p.db.WithContext(ctx).Create(&slot).Error; err != nil {
		_ = p.provider.DeleteSandbox(context.WithoutCancel(ctx), info.ExternalID)
		return uuid.Nil, err
	}
	return slot.ID, nil
}

func (p *WarmPool) deleteStaleAvailableSlots(ctx context.Context, mode, image string) error {
	var stale []model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).
		Where("provider_id = ? AND mode = ? AND runtime_image <> ? AND status IN ?", p.provider.ID(), mode, image, []string{
			model.SandboxWarmSlotStatusWarm,
			model.SandboxWarmSlotStatusWarming,
		}).
		Find(&stale).Error; err != nil {
		return err
	}
	if len(stale) == 0 {
		return nil
	}
	logger := logging.FromContext(ctx)
	for _, slot := range stale {
		logger.InfoContext(ctx, "deleting stale sandbox warm slot",
			"provider", p.provider.ID(), "mode", mode, "slot_id", slot.ID,
			"external_id", slot.ExternalID, "runtime_image", slot.RuntimeImage,
			"expected_runtime_image", image)
		// MarkError owns provider deletion (P0-19), so delegate to it rather than
		// deleting here to avoid a double delete.
		if err := p.MarkError(ctx, slot.ID, fmt.Sprintf("stale runtime image %s; expected %s", slot.RuntimeImage, image)); err != nil {
			return fmt.Errorf("delete stale warm slot %s: %w", slot.ExternalID, err)
		}
	}
	return nil
}

func (p *WarmPool) runtimeImage(mode string) string {
	if mode == model.SandboxWarmSlotModeSpecialist {
		return strings.TrimSpace(p.cfg.SandboxesRuntimeSpecialistImage)
	}
	return strings.TrimSpace(p.cfg.SandboxesRuntimeBaseImage)
}

func (p *WarmPool) slotName(mode string) string {
	return fmt.Sprintf("hivy-%s-warm-%s", mode, strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
}

func (p *WarmPool) warmSlotEnvVars() map[string]string {
	envVars := map[string]string{}
	if p == nil || p.cfg == nil {
		return envVars
	}
	setSandboxSentryEnvVars(envVars, p.cfg, p.cfg.AgentSandboxSentryDSN)
	return envVars
}

func containsUUID(items []uuid.UUID, target uuid.UUID) bool {
	for _, item := range items {
		if item == target {
			return true
		}
	}
	return false
}
