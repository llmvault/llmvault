package model

import (
	"time"

	"github.com/google/uuid"
)

const (
	CreditPurchasePending  = "pending"
	CreditPurchasePaid     = "paid"
	CreditPurchaseCredited = "credited"
	CreditPurchaseFailed   = "failed"
	CreditPurchaseReversed = "reversed"
	CreditPurchaseRefunded = "refunded"
)

// CreditPurchase is the durable financial record for a one-time Paystack
// deposit. The credit ledger references its ID after payment verification.
type CreditPurchase struct {
	ID                 uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID              uuid.UUID  `gorm:"type:uuid;not null;index"`
	CreatedByUserID    *uuid.UUID `gorm:"type:uuid;index"`
	PackID             string     `gorm:"not null;size:32"`
	IdempotencyKey     string     `gorm:"not null;size:64"`
	PaymentMethodID    *uuid.UUID `gorm:"type:uuid;index"`
	SavePaymentMethod  bool       `gorm:"not null;default:false"`
	Provider           string     `gorm:"not null;size:32"`
	ProviderRef        string     `gorm:"column:provider_reference;not null;default:'';size:128"`
	CheckoutAccessCode string     `gorm:"not null;default:'';size:128"`
	CheckoutURL        string     `gorm:"not null;default:'';type:text"`
	Status             string     `gorm:"not null;default:'pending';size:32"`
	Currency           string     `gorm:"not null;size:3"`
	SubtotalMinor      int64      `gorm:"not null"`
	FeeBasisPoints     int64      `gorm:"not null"`
	FeeMinor           int64      `gorm:"not null"`
	TotalMinor         int64      `gorm:"not null"`
	Credits            int64      `gorm:"not null"`
	FXMinorPerUSD      *int64     `gorm:"column:fx_minor_per_usd"`
	ProviderPaid       int64      `gorm:"column:provider_paid_minor;not null;default:0"`
	ProviderCurrency   string     `gorm:"column:provider_paid_currency;not null;default:'';size:3"`
	PaidAt             *time.Time
	CreditedAt         *time.Time
	FailedAt           *time.Time
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

func (CreditPurchase) TableName() string { return "credit_purchases" }
