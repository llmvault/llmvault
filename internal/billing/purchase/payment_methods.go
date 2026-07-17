package purchase

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
)

type paymentMethodSecret struct {
	Authorization billing.PaymentAuthorization `json:"authorization"`
	Email         string                       `json:"email"`
}

func (s *Service) ListPaymentMethods(ctx context.Context, orgID, userID uuid.UUID) ([]model.BillingPaymentMethod, error) {
	var methods []model.BillingPaymentMethod
	if err := s.db.WithContext(ctx).Where("org_id = ? AND user_id = ?", orgID, userID).
		Order("created_at DESC").Find(&methods).Error; err != nil {
		return nil, fmt.Errorf("list payment methods: %w", err)
	}
	return methods, nil
}

func (s *Service) DeletePaymentMethod(ctx context.Context, orgID, userID, methodID uuid.UUID) error {
	res := s.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND user_id = ?", methodID, orgID, userID).
		Delete(&model.BillingPaymentMethod{})
	if res.Error != nil {
		return fmt.Errorf("delete payment method: %w", res.Error)
	}
	if res.RowsAffected != 1 {
		return ErrPaymentMethodNotFound
	}
	return nil
}

func (s *Service) loadPaymentMethodSecret(ctx context.Context, orgID, userID, methodID uuid.UUID) (*model.BillingPaymentMethod, *paymentMethodSecret, error) {
	if s.kms == nil {
		return nil, nil, ErrPaymentMethodUnavailable
	}
	var method model.BillingPaymentMethod
	if err := s.db.WithContext(ctx).Where("id = ? AND org_id = ? AND user_id = ?", methodID, orgID, userID).First(&method).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil, ErrPaymentMethodNotFound
		}
		return nil, nil, fmt.Errorf("load payment method: %w", err)
	}
	dek, err := s.kms.Unwrap(ctx, method.WrappedDEK)
	if err != nil {
		return nil, nil, fmt.Errorf("unwrap payment method key: %w", err)
	}
	plaintext, err := crypto.DecryptCredential(method.EncryptedAuthorization, dek)
	if err != nil {
		return nil, nil, fmt.Errorf("decrypt payment method: %w", err)
	}
	var secret paymentMethodSecret
	if err := json.Unmarshal(plaintext, &secret); err != nil {
		return nil, nil, fmt.Errorf("decode payment method: %w", err)
	}
	if secret.Authorization.AuthorizationCode == "" || !secret.Authorization.Reusable || secret.Email == "" {
		return nil, nil, ErrPaymentMethodUnavailable
	}
	return &method, &secret, nil
}

func (s *Service) savePaymentMethod(ctx context.Context, purchase model.CreditPurchase, result *billing.DepositResult) error {
	if s.kms == nil || purchase.CreatedByUserID == nil || result.Authorization == nil {
		return nil
	}
	authorization := *result.Authorization
	if !authorization.Reusable || authorization.Channel != "card" || authorization.AuthorizationCode == "" || authorization.Signature == "" || result.CustomerEmail == "" {
		return nil
	}
	payload, err := json.Marshal(paymentMethodSecret{Authorization: authorization, Email: result.CustomerEmail})
	if err != nil {
		return fmt.Errorf("encode payment method: %w", err)
	}
	dek, err := crypto.GenerateDEK()
	if err != nil {
		return fmt.Errorf("generate payment method key: %w", err)
	}
	encrypted, err := crypto.EncryptCredential(payload, dek)
	if err != nil {
		return fmt.Errorf("encrypt payment method: %w", err)
	}
	wrapped, err := s.kms.Wrap(ctx, dek)
	if err != nil {
		return fmt.Errorf("wrap payment method key: %w", err)
	}
	method := model.BillingPaymentMethod{
		OrgID:                  purchase.OrgID,
		UserID:                 *purchase.CreatedByUserID,
		Provider:               providerName,
		ProviderSignature:      authorization.Signature,
		EncryptedAuthorization: encrypted,
		WrappedDEK:             wrapped,
		CardType:               strings.TrimSpace(authorization.CardType),
		Last4:                  authorization.Last4,
		ExpMonth:               authorization.ExpMonth,
		ExpYear:                authorization.ExpYear,
		Bank:                   strings.TrimSpace(authorization.Bank),
		CountryCode:            authorization.CountryCode,
	}
	return s.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "org_id"}, {Name: "user_id"}, {Name: "provider"}, {Name: "provider_signature"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"encrypted_authorization", "wrapped_dek", "card_type", "last4",
			"exp_month", "exp_year", "bank", "country_code", "updated_at",
		}),
	}).Create(&method).Error
}
