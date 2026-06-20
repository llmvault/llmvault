package sandbox

import (
	"context"
	"fmt"
	"hash/fnv"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

func (p *WarmPool) Reconcile(ctx context.Context, profile WarmPoolProfile, onCreated func(context.Context, uuid.UUID) error) ([]uuid.UUID, error) {
	if p == nil {
		return nil, nil
	}
	profile = normalizeWarmPoolProfile(p.cfg, profile)
	desired := p.DesiredCount(profile)
	if desired <= 0 {
		return nil, nil
	}
	if profile.RuntimeImage == "" {
		return nil, fmt.Errorf("runtime image for warm %s sandbox is not configured", profile.Mode)
	}

	// Serialise reconciles for this warm profile across pods via a tx-scoped
	// advisory lock. Without it the count-then-provision below races: two reconciles
	// each read 'available < desired' and both provision, over-provisioning forever.
	var created []uuid.UUID
	err := p.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(?)", warmPoolReconcileLockKey(p.provider.ID(), profile)).Error; err != nil {
			return fmt.Errorf("acquire warm pool reconcile lock: %w", err)
		}
		var rerr error
		created, rerr = p.reconcileLocked(ctx, tx, profile, desired, onCreated)
		return rerr
	})
	return created, err
}

func (p *WarmPool) reconcileLocked(ctx context.Context, tx *gorm.DB, profile WarmPoolProfile, desired int, onCreated func(context.Context, uuid.UUID) error) ([]uuid.UUID, error) {
	logger := logging.FromContext(ctx)
	if err := p.deleteStaleAvailableSlots(ctx, profile); err != nil {
		return nil, err
	}

	var availableSlots []model.SandboxWarmSlot
	if err := tx.WithContext(ctx).
		Where(
			"provider_id = ? AND mode = ? AND image_kind = ? AND runtime_image = ? AND sandbox_size = ? AND cpu = ? AND memory = ? AND disk = ? AND status IN ?",
			p.provider.ID(), profile.Mode, profile.ImageKind, profile.RuntimeImage, profile.SandboxSize, profile.CPU, profile.Memory, profile.Disk,
			[]string{model.SandboxWarmSlotStatusWarm, model.SandboxWarmSlotStatusWarming},
		).
		Order("created_at ASC").
		Find(&availableSlots).Error; err != nil {
		return nil, err
	}
	available := int64(len(availableSlots))
	logger.InfoContext(ctx, "sandbox warm pool reconcile",
		"provider", p.provider.ID(), "mode", profile.Mode, "desired", desired, "available", available,
		"image_kind", profile.ImageKind, "sandbox_size", profile.SandboxSize, "runtime_image", profile.RuntimeImage,
		"cpu", profile.CPU, "memory", profile.Memory, "disk", profile.Disk)

	// Scale down: trim surplus slots so a lowered pool size (or transient
	// over-provision) releases the extra paid services rather than running forever.
	if int(available) > desired {
		surplus := int(available) - desired
		if err := p.trimSurplusSlots(ctx, availableSlots, surplus); err != nil {
			return nil, err
		}
		return nil, nil
	}

	created := make([]uuid.UUID, 0, desired-int(available))
	for i := available; i < int64(desired); i++ {
		slotID, err := p.provision(ctx, profile)
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
		Where(
			"provider_id = ? AND mode = ? AND image_kind = ? AND runtime_image = ? AND sandbox_size = ? AND cpu = ? AND memory = ? AND disk = ? AND status = ?",
			p.provider.ID(), profile.Mode, profile.ImageKind, profile.RuntimeImage, profile.SandboxSize, profile.CPU, profile.Memory, profile.Disk,
			model.SandboxWarmSlotStatusWarming,
		).
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
// so the provider resource is released rather than just flipping the row.
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

// warmPoolReconcileLockKey derives a stable bigint advisory-lock key for a warm profile.
// Collisions only ever serialise unrelated reconciles, which is harmless.
func warmPoolReconcileLockKey(providerID string, profile WarmPoolProfile) int64 {
	h := fnv.New64a()
	_, _ = h.Write([]byte("warm-pool-reconcile:"))
	_, _ = h.Write([]byte(providerID))
	_, _ = h.Write([]byte{0})
	_, _ = h.Write([]byte(profile.logValue()))
	return int64(h.Sum64()) // #nosec G115 -- hash truncation; sign bit is part of the hash distribution
}

func (p *WarmPool) provision(ctx context.Context, profile WarmPoolProfile) (uuid.UUID, error) {
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
	logger := logging.FromContext(ctx)
	logger.InfoContext(ctx, "sandbox warm slot provisioning",
		"provider", p.provider.ID(), "mode", profile.Mode, "image_kind", profile.ImageKind,
		"sandbox_size", profile.SandboxSize, "image", profile.RuntimeImage, "port", p.cfg.RailwayRuntimePort,
		"cpu", profile.CPU, "memory", profile.Memory, "disk", profile.Disk)
	info, err := provider.CreateWarmSlot(ctx, WarmSlotCreateOpts{
		Name:          p.slotName(profile),
		Mode:          profile.Mode,
		ImageKind:     profile.ImageKind,
		SandboxSize:   profile.SandboxSize,
		RuntimeImage:  profile.RuntimeImage,
		RuntimePort:   p.cfg.RailwayRuntimePort,
		CPU:           profile.CPU,
		Memory:        profile.Memory,
		Disk:          profile.Disk,
		RuntimeSecret: runtimeSecret,
		EnvVars:       p.warmSlotEnvVars(),
		Labels: map[string]string{
			"mode":          profile.Mode,
			"provider":      p.provider.ID(),
			"sandbox_image": profile.ImageKind,
			"sandbox_size":  profile.SandboxSize,
		},
	})
	if err != nil {
		return uuid.Nil, err
	}
	logger.InfoContext(ctx, "sandbox warm slot provider resource created",
		"provider", p.provider.ID(), "mode", profile.Mode, "external_id", info.ExternalID,
		"endpoint_url", info.EndpointURL, "port", info.RuntimePort)
	slot := model.SandboxWarmSlot{
		ProviderID:             p.provider.ID(),
		Mode:                   profile.Mode,
		Status:                 model.SandboxWarmSlotStatusWarming,
		ExternalID:             info.ExternalID,
		EndpointURL:            info.EndpointURL,
		RuntimeImage:           profile.RuntimeImage,
		ImageKind:              profile.ImageKind,
		SandboxSize:            profile.SandboxSize,
		CPU:                    profile.CPU,
		Memory:                 profile.Memory,
		Disk:                   profile.Disk,
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

func (p *WarmPool) deleteStaleAvailableSlots(ctx context.Context, profile WarmPoolProfile) error {
	var candidates []model.SandboxWarmSlot
	if err := p.db.WithContext(ctx).
		Where(
			"provider_id = ? AND mode = ? AND image_kind = ? AND sandbox_size = ? AND cpu = ? AND memory = ? AND disk = ? AND status IN ?",
			p.provider.ID(), profile.Mode, profile.ImageKind, profile.SandboxSize, profile.CPU, profile.Memory, profile.Disk,
			[]string{model.SandboxWarmSlotStatusWarm, model.SandboxWarmSlotStatusWarming},
		).
		Find(&candidates).Error; err != nil {
		return err
	}
	stale := make([]model.SandboxWarmSlot, 0, len(candidates))
	for _, slot := range candidates {
		if slot.RuntimeImage == profile.RuntimeImage {
			continue
		}
		if imageRepository(profile.RuntimeImage) != "" && imageRepository(slot.RuntimeImage) != imageRepository(profile.RuntimeImage) {
			continue
		}
		stale = append(stale, slot)
	}
	if len(stale) == 0 {
		return nil
	}
	logger := logging.FromContext(ctx)
	for _, slot := range stale {
		logger.InfoContext(ctx, "deleting stale sandbox warm slot",
			"provider", p.provider.ID(), "mode", profile.Mode, "slot_id", slot.ID,
			"external_id", slot.ExternalID, "runtime_image", slot.RuntimeImage,
			"expected_runtime_image", profile.RuntimeImage)
		// MarkError owns provider deletion, so delegate to it rather than
		// deleting here to avoid a double delete.
		if err := p.MarkError(ctx, slot.ID, fmt.Sprintf("stale runtime image %s; expected %s", slot.RuntimeImage, profile.RuntimeImage)); err != nil {
			return fmt.Errorf("delete stale warm slot %s: %w", slot.ExternalID, err)
		}
	}
	return nil
}

func (p *WarmPool) slotName(profile WarmPoolProfile) string {
	return fmt.Sprintf("hivy-%s-%s-%s-warm-%s", profile.Mode, profile.ImageKind, profile.SandboxSize, strings.ReplaceAll(uuid.NewString()[:8], "-", ""))
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
