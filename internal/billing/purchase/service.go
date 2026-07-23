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
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/orgtier"
)

const (
	DepositFeeBasisPoints int64 = 1200
	WelcomeCredits        int64 = 1000
	// NGNMinorPerUSD is the fixed product conversion rate in kobo per US dollar.
	NGNMinorPerUSD int64 = 145_000
	providerName         = "paystack"
)

var (
	ErrInvalidCurrency            = errors.New("credit purchase: unsupported billing currency")
	ErrInvalidAmount              = errors.New("credit purchase: invalid deposit amount")
	ErrNotFound                   = errors.New("credit purchase: not found")
	ErrPaymentPending             = errors.New("credit purchase: payment is not complete")
	ErrPaymentMismatch            = errors.New("credit purchase: paid amount or currency does not match")
	ErrInvalidPack                = errors.New("credit purchase: invalid credit pack")
	ErrInvalidRequestKey          = errors.New("credit purchase: invalid idempotency key")
	ErrPaymentMethodNotFound      = errors.New("credit purchase: payment method not found")
	ErrPaymentMethodUnavailable   = errors.New("credit purchase: saved payment method is unavailable")
	ErrPaymentCurrencyUnavailable = errors.New("credit purchase: payment currency is unavailable")
)

type Service struct {
	db       *gorm.DB
	registry *billing.Registry
	credits  *billing.CreditsService
	kms      *crypto.KeyWrapper
}

func NewService(db *gorm.DB, registry *billing.Registry, credits *billing.CreditsService, kms *crypto.KeyWrapper) *Service {
	return &Service{db: db, registry: registry, credits: credits, kms: kms}
}

type CreateInput struct {
	OrgID             uuid.UUID
	UserID            uuid.UUID
	Email             string
	Currency          billing.Currency
	PackID            string
	SubtotalMinor     *int64
	IdempotencyKey    string
	PaymentMethodID   *uuid.UUID
	SavePaymentMethod bool
}

type CreateResult struct {
	Purchase *model.CreditPurchase
	Session  *billing.DepositSession
}

func (s *Service) Create(ctx context.Context, in CreateInput) (*CreateResult, error) {
	requestKey, err := uuid.Parse(in.IdempotencyKey)
	if err != nil || requestKey == uuid.Nil {
		return nil, ErrInvalidRequestKey
	}
	var org model.Org
	if err := s.db.WithContext(ctx).Where("id = ?", in.OrgID).First(&org).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("load org: %w", err)
	}
	currency := in.Currency
	if !currency.IsValid() {
		return nil, ErrInvalidCurrency
	}
	packID, subtotalMinor, err := resolvePurchaseAmount(in.PackID, in.SubtotalMinor, currency)
	if err != nil {
		return nil, err
	}
	var existing model.CreditPurchase
	if err := s.db.WithContext(ctx).Where("org_id = ? AND idempotency_key = ?", in.OrgID, requestKey.String()).First(&existing).Error; err == nil {
		return createResultFromPurchase(&existing), nil
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("load idempotent purchase: %w", err)
	}

	credits, fxMinorPerUSD, err := s.creditsForSubtotal(currency, subtotalMinor)
	if err != nil {
		return nil, err
	}
	fee, err := percentageCeil(subtotalMinor, DepositFeeBasisPoints)
	if err != nil || subtotalMinor > math.MaxInt64-fee {
		return nil, ErrInvalidAmount
	}
	purchaseID := uuid.New()
	purchase := &model.CreditPurchase{
		ID:                purchaseID,
		OrgID:             in.OrgID,
		CreatedByUserID:   &in.UserID,
		PackID:            packID,
		IdempotencyKey:    requestKey.String(),
		PaymentMethodID:   in.PaymentMethodID,
		SavePaymentMethod: in.SavePaymentMethod,
		Provider:          providerName,
		ProviderRef:       purchaseID.String(),
		Status:            model.CreditPurchasePending,
		Currency:          string(currency),
		SubtotalMinor:     subtotalMinor,
		FeeBasisPoints:    DepositFeeBasisPoints,
		FeeMinor:          fee,
		TotalMinor:        subtotalMinor + fee,
		Credits:           credits,
		FXMinorPerUSD:     fxMinorPerUSD,
	}
	res := s.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(purchase)
	if res.Error != nil {
		return nil, fmt.Errorf("create credit purchase: %w", res.Error)
	}
	if res.RowsAffected == 0 {
		if err := s.db.WithContext(ctx).Where("org_id = ? AND idempotency_key = ?", in.OrgID, requestKey.String()).First(&existing).Error; err != nil {
			return nil, fmt.Errorf("load concurrent purchase: %w", err)
		}
		return createResultFromPurchase(&existing), nil
	}
	provider, err := s.registry.Get(providerName)
	if err != nil {
		s.markFailed(ctx, purchase.ID)
		return nil, err
	}
	metadata := map[string]string{"org_id": in.OrgID.String(), "purchase_id": purchase.ID.String(), "pack_id": packID}
	var session *billing.DepositSession
	if in.PaymentMethodID != nil {
		_, secret, loadErr := s.loadPaymentMethodSecret(ctx, in.OrgID, in.UserID, *in.PaymentMethodID, currency)
		if loadErr != nil {
			s.markFailed(ctx, purchase.ID)
			return nil, loadErr
		}
		session, err = provider.ChargeSavedPayment(ctx, billing.SavedPaymentCharge{
			PurchaseID: purchase.ID, OrgID: in.OrgID,
			AuthorizationCode: secret.Authorization.AuthorizationCode,
			CustomerEmail:     secret.Email, AmountMinor: purchase.TotalMinor,
			Currency: currency, Metadata: metadata,
		})
	} else {
		session, err = provider.CreateDeposit(ctx, billing.DepositIntent{
			PurchaseID: purchase.ID, OrgID: in.OrgID, CustomerEmail: in.Email,
			AmountMinor: purchase.TotalMinor, Currency: currency,
			Metadata: metadata,
			Channels: checkoutChannels(currency),
		})
	}
	if err != nil {
		s.markFailed(ctx, purchase.ID)
		var providerErr *billing.ProviderRequestError
		if errors.As(err, &providerErr) &&
			providerErr.StatusCode == 403 &&
			providerErr.Type == "validation_error" {
			return nil, fmt.Errorf("%w: %w", ErrPaymentCurrencyUnavailable, err)
		}
		return nil, fmt.Errorf("initialize deposit: %w", err)
	}
	if session.Reference != purchase.ID.String() {
		s.markFailed(ctx, purchase.ID)
		return nil, ErrPaymentMismatch
	}
	res = s.db.WithContext(ctx).Model(&model.CreditPurchase{}).
		Where("id = ? AND org_id = ?", purchase.ID, in.OrgID).
		Updates(map[string]any{
			"provider_reference":   session.Reference,
			"checkout_access_code": session.AccessCode,
			"checkout_url":         session.URL,
		})
	if res.Error != nil {
		return nil, fmt.Errorf("store deposit reference: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return nil, ErrNotFound
	}
	purchase.ProviderRef = session.Reference
	purchase.CheckoutAccessCode = session.AccessCode
	purchase.CheckoutURL = session.URL
	return &CreateResult{Purchase: purchase, Session: session}, nil
}

func createResultFromPurchase(purchase *model.CreditPurchase) *CreateResult {
	reference := purchase.ProviderRef
	if reference == "" {
		reference = purchase.ID.String()
	}
	return &CreateResult{Purchase: purchase, Session: &billing.DepositSession{
		Reference: reference, AccessCode: purchase.CheckoutAccessCode, URL: purchase.CheckoutURL,
	}}
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
	if result.Reference != purchase.ProviderRef || result.PaidAmountMinor != purchase.TotalMinor || result.Currency != billing.Currency(purchase.Currency) {
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
		if err := orgtier.PromoteForCompletedDeposits(ctx, tx, orgID); err != nil {
			return err
		}
		if err := tx.Where("id = ?", fresh.ID).First(&purchase).Error; err != nil {
			return err
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("credit deposit: %w", err)
	}
	if purchase.SavePaymentMethod {
		if err := s.savePaymentMethod(ctx, purchase, result); err != nil {
			logging.FromContext(ctx).ErrorContext(ctx, "save billing payment method", "error", err, "purchase_id", purchase.ID)
		}
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
		if subtotal > math.MaxInt64/1000 {
			return 0, nil, ErrInvalidAmount
		}
		credits := subtotal * 1000 / NGNMinorPerUSD
		if credits <= 0 {
			return 0, nil, ErrInvalidAmount
		}
		rate := NGNMinorPerUSD
		return credits, &rate, nil
	default:
		return 0, nil, ErrInvalidCurrency
	}
}

// Omitting channels lets Paystack offer every checkout method enabled for the
// merchant and supported by the transaction currency. USD remains card-only.
func checkoutChannels(currency billing.Currency) []string {
	if currency == billing.CurrencyUSD {
		return []string{"card"}
	}
	return nil
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
