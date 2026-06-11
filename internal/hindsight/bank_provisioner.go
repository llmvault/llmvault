package hindsight

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

var ErrBankNotProvisioned = errors.New("hindsight bank is not provisioned")

type BankProvisioner struct {
	db     *gorm.DB
	client *Client
}

func NewBankProvisioner(db *gorm.DB, client *Client) *BankProvisioner {
	if db == nil || client == nil {
		return nil
	}
	return &BankProvisioner{db: db, client: client}
}

func (p *BankProvisioner) EnsureOrgBank(ctx context.Context, orgID uuid.UUID) error {
	if p == nil || p.db == nil || p.client == nil {
		return nil
	}
	if orgID == uuid.Nil {
		return fmt.Errorf("ensure hindsight org bank: org_id is required")
	}
	bankID := OrgBankID(orgID)
	cfg := DefaultMemoryConfig()
	hash := OrgBankConfigHash(orgID, cfg)

	var bank model.HindsightBank
	err := p.db.WithContext(ctx).Where("bank_id = ?", bankID).First(&bank).Error
	if err == nil && bank.ConfigHash == hash {
		return nil
	}
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("check hindsight org bank: %w", err)
	}

	if err := p.client.ConfigureBank(ctx, bankID, cfg.ToBankConfigUpdate()); err != nil {
		// Bank may not exist yet on the Hindsight side. A retain call with an
		// empty items list auto-provisions it, then we retry the configure.
		if _, retainErr := p.client.Retain(ctx, bankID, &RetainRequest{Items: []RetainItem{}, Async: true}); retainErr != nil {
			return fmt.Errorf("configure hindsight org bank: %w (retain fallback: %v)", err, retainErr)
		}
		if retryErr := p.client.ConfigureBank(ctx, bankID, cfg.ToBankConfigUpdate()); retryErr != nil {
			return fmt.Errorf("configure hindsight org bank after provisioning: %w", retryErr)
		}
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if createErr := p.client.CreateMentalModel(ctx, bankID, &CreateMentalModelRequest{
			Name:        "Organization Memory",
			SourceQuery: "Summarize everything known across all agents in this organization.",
			Trigger:     &MentalModelTrigger{RefreshAfterConsolidation: true},
		}); createErr != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("create hindsight org mental model: %w", createErr), map[string]any{
				"org_id":  orgID.String(),
				"bank_id": bankID,
			})
		}
	}

	row := model.HindsightBank{BankID: bankID, ConfigHash: hash}
	if err := p.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "bank_id"}},
		DoUpdates: clause.AssignmentColumns([]string{"config_hash", "updated_at"}),
	}).Create(&row).Error; err != nil {
		return fmt.Errorf("record hindsight org bank: %w", err)
	}
	logging.FromContext(ctx).InfoContext(ctx, "hindsight org bank ensured",
		"org_id", orgID.String(),
		"bank_id", bankID,
		"created", errors.Is(err, gorm.ErrRecordNotFound),
	)
	return nil
}

func OrgBankConfigHash(orgID uuid.UUID, cfg MemoryConfig) string {
	sum := sha256.Sum256([]byte(cfg.Hash() + "|org-" + orgID.String()))
	return hex.EncodeToString(sum[:])
}

func BankExists(ctx context.Context, db *gorm.DB, bankID string) (bool, error) {
	if db == nil {
		return true, nil
	}
	var count int64
	if err := db.WithContext(ctx).Model(&model.HindsightBank{}).Where("bank_id = ?", bankID).Count(&count).Error; err != nil {
		return false, fmt.Errorf("check hindsight bank: %w", err)
	}
	return count > 0, nil
}

func RequireBank(ctx context.Context, db *gorm.DB, bankID string) error {
	exists, err := BankExists(ctx, db, bankID)
	if err != nil {
		return err
	}
	if !exists {
		return fmt.Errorf("%w: %s", ErrBankNotProvisioned, bankID)
	}
	return nil
}
