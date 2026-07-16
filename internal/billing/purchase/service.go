package purchase

import (
	"context"
	"errors"
	"fmt"
	"math"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

const (
	DepositFeeBasisPoints int64 = 1000
	WelcomeCredits        int64 = 1000
	providerName                = "paystack"
)

var (
	ErrCurrencyRequired = errors.New("credit purchase: billing currency is required")
	ErrCurrencyLocked   = errors.New("credit purchase: billing currency is already locked")
	ErrInvalidCurrency  = errors.New("credit purchase: unsupported billing currency")
	ErrInvalidAmount    = errors.New("credit purchase: invalid deposit amount")
	ErrNotFound         = errors.New("credit purchase: not found")
	ErrPaymentPending   = errors.New("credit purchase: payment is not complete")
	ErrPaymentMismatch  = errors.New("credit purchase: paid amount or currency does not match")
)

type Service struct {
	db             *gorm.DB
	registry       *billing.Registry
	credits        *billing.CreditsService
	ngnMinorPerUSD int64
}

func NewService(db *gorm.DB, registry *billing.Registry, credits *billing.CreditsService, ngnMinorPerUSD int64) *Service {
	return &Service{db: db, registry: registry, credits: credits, ngnMinorPerUSD: ngnMinorPerUSD}
}

func (s *Service) NGNMinorPerUSD() int64 { return s.ngnMinorPerUSD }

func (s *Service) SelectCurrency(ctx context.Context, orgID uuid.UUID, currency billing.Currency) error {
	if !currency.IsValid() {
		return ErrInvalidCurrency
	}
	res := s.db.WithContext(ctx).Model(&model.Org{}).
		Where("id = ? AND billing_currency = ''", orgID).
		Update("billing_currency", string(currency))
	if res.Error != nil {
		return fmt.Errorf("select billing currency: %w", res.Error)
	}
	if res.RowsAffected == 1 {
		return nil
	}
	var org model.Org
	if err := s.db.WithContext(ctx).Where("id = ?", orgID).First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return ErrNotFound
		}
		return fmt.Errorf("load org billing currency: %w", err)
	}
	if org.BillingCurrency == string(currency) {
		return nil
	}
	return ErrCurrencyLocked
}

type CreateInput struct {
	OrgID         uuid.UUID
	UserID        uuid.UUID
	Email         string
	SubtotalMinor int64
	CallbackURL   string
}

type CreateResult struct {
	Purchase *model.CreditPurchase
	Session  *billing.DepositSession
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*CreateResult, error) {
	var org model.Org
	if err := s.db.WithContext(ctx).Where("id = ?", in.OrgID).First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load org: %w", err)
	}
	currency := billing.Currency(org.BillingCurrency)
	if org.BillingCurrency == "" {
		return nil, ErrCurrencyRequired
	}
	if !currency.IsValid() {
		return nil, ErrInvalidCurrency
	}
	credits, fxMinorPerUSD, err := s.creditsForSubtotal(currency, in.SubtotalMinor)
	if err != nil {
		return nil, err
	}
	fee, err := percentageCeil(in.SubtotalMinor, DepositFeeBasisPoints)
	if err != nil || in.SubtotalMinor > math.MaxInt64-fee {
		return nil, ErrInvalidAmount
	}
	purchase := &model.CreditPurchase{
		OrgID:           in.OrgID,
		CreatedByUserID: &in.UserID,
		Provider:        providerName,
		Status:          model.CreditPurchasePending,
		Currency:        string(currency),
		SubtotalMinor:   in.SubtotalMinor,
		FeeBasisPoints:  DepositFeeBasisPoints,
		FeeMinor:        fee,
		TotalMinor:      in.SubtotalMinor + fee,
		Credits:         credits,
		FXMinorPerUSD:   fxMinorPerUSD,
	}
	if err := s.db.WithContext(ctx).Create(purchase).Error; err != nil {
		return nil, fmt.Errorf("create credit purchase: %w", err)
	}
	provider, err := s.registry.Get(providerName)
	if err != nil {
		s.markFailed(ctx, purchase.ID)
		return nil, err
	}
	session, err := provider.CreateDeposit(ctx, billing.DepositIntent{
		PurchaseID:    purchase.ID,
		OrgID:         in.OrgID,
		CustomerEmail: in.Email,
		AmountMinor:   purchase.TotalMinor,
		Currency:      currency,
		CallbackURL:   in.CallbackURL,
		Metadata: map[string]string{
			"org_id":      in.OrgID.String(),
			"purchase_id": purchase.ID.String(),
		},
	})
	if err != nil {
		s.markFailed(ctx, purchase.ID)
		return nil, fmt.Errorf("initialize deposit: %w", err)
	}
	res := s.db.WithContext(ctx).Model(&model.CreditPurchase{}).
		Where("id = ? AND org_id = ?", purchase.ID, in.OrgID).
		Update("provider_reference", session.Reference)
	if res.Error != nil {
		return nil, fmt.Errorf("store deposit reference: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return nil, ErrNotFound
	}
	purchase.ProviderRef = session.Reference
	return &CreateResult{Purchase: purchase, Session: session}, nil
}

func (s *Service) Verify(ctx context.Context, orgID, purchaseID uuid.UUID) (*model.CreditPurchase, error) {
	var purchase model.CreditPurchase
	if err := s.db.WithContext(ctx).Where("id = ? AND org_id = ?", purchaseID, orgID).First(&purchase).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load credit purchase: %w", err)
	}
	if purchase.Status == model.CreditPurchaseCredited {
		return &purchase, nil
	}
	provider, err := s.registry.Get(purchase.Provider)
	if err != nil {
		return nil, err
	}
	result, err := provider.ResolveDeposit(ctx, billing.ResolveDepositRequest{
		Reference:          purchase.ProviderRef,
		ExpectedOrgID:      orgID,
		ExpectedPurchaseID: purchaseID,
	})
	if err != nil {
		return nil, fmt.Errorf("resolve deposit: %w", err)
	}
	if result.Status != billing.PaymentPaid {
		return nil, ErrPaymentPending
	}
	if result.PaidAmountMinor != purchase.TotalMinor || result.Currency != billing.Currency(purchase.Currency) {
		return nil, ErrPaymentMismatch
	}

	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var fresh model.CreditPurchase
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ? AND org_id = ?", purchaseID, orgID).First(&fresh).Error; err != nil {
			return err
		}
		if fresh.Status == model.CreditPurchaseCredited {
			purchase = fresh
			return nil
		}
		paidAt := time.Now().UTC()
		if result.PaidAt != nil {
			paidAt = result.PaidAt.UTC()
		}
		if err := billing.GrantWithTx(tx, orgID, fresh.Credits, billing.ReasonTopup, "credit_purchase", fresh.ID.String()); err != nil && !errors.Is(err, billing.ErrAlreadyRecorded) {
			return fmt.Errorf("grant purchased credits: %w", err)
		}
		now := time.Now().UTC()
		updates := map[string]any{
			"status":                 model.CreditPurchaseCredited,
			"provider_paid_minor":    result.PaidAmountMinor,
			"provider_paid_currency": string(result.Currency),
			"paid_at":                paidAt,
			"credited_at":            now,
		}
		res := tx.Model(&model.CreditPurchase{}).Where("id = ? AND org_id = ?", fresh.ID, orgID).Updates(updates)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected != 1 {
			return ErrNotFound
		}
		if err := tx.Where("id = ?", fresh.ID).First(&purchase).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("credit deposit: %w", err)
	}
	return &purchase, nil
}

func (s *Service) List(ctx context.Context, orgID uuid.UUID, limit int) ([]model.CreditPurchase, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	var purchases []model.CreditPurchase
	if err := s.db.WithContext(ctx).Where("org_id = ?", orgID).
		Order("created_at DESC").Limit(limit).Find(&purchases).Error; err != nil {
		return nil, fmt.Errorf("list credit purchases: %w", err)
	}
	return purchases, nil
}

func (s *Service) creditsForSubtotal(currency billing.Currency, subtotal int64) (int64, *int64, error) {
	if subtotal <= 0 {
		return 0, nil, ErrInvalidAmount
	}
	switch currency {
	case billing.CurrencyUSD:
		if subtotal > math.MaxInt64/10 {
			return 0, nil, ErrInvalidAmount
		}
		return subtotal * 10, nil, nil
	case billing.CurrencyNGN:
		if s.ngnMinorPerUSD <= 0 || subtotal > math.MaxInt64/1000 {
			return 0, nil, ErrInvalidAmount
		}
		credits := subtotal * 1000 / s.ngnMinorPerUSD
		if credits <= 0 {
			return 0, nil, ErrInvalidAmount
		}
		rate := s.ngnMinorPerUSD
		return credits, &rate, nil
	default:
		return 0, nil, ErrInvalidCurrency
	}
}

func percentageCeil(amount, basisPoints int64) (int64, error) {
	if amount <= 0 || basisPoints < 0 {
		return 0, ErrInvalidAmount
	}
	if basisPoints == 0 {
		return 0, nil
	}
	if amount > math.MaxInt64/basisPoints {
		return 0, ErrInvalidAmount
	}
	product := amount * basisPoints
	return (product + 9_999) / 10_000, nil
}

func (s *Service) markFailed(ctx context.Context, purchaseID uuid.UUID) {
	now := time.Now().UTC()
	_ = s.db.WithContext(ctx).Model(&model.CreditPurchase{}).Where("id = ?", purchaseID).
		Updates(map[string]any{"status": model.CreditPurchaseFailed, "failed_at": now}).Error // best-effort failure bookkeeping
}
