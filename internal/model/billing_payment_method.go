package model

import (
	"time"

	"github.com/google/uuid"
)

// BillingPaymentMethod stores an encrypted Paystack reusable authorization.
// Only non-sensitive display fields are kept in plaintext; card details never
// pass through or persist in Hivy.
type BillingPaymentMethod struct {
	ID                     uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID                  uuid.UUID `gorm:"type:uuid;not null;index"`
	UserID                 uuid.UUID `gorm:"type:uuid;not null;index"`
	Provider               string    `gorm:"not null;size:32"`
	ProviderSignature      string    `gorm:"not null;size:128"`
	Currency               string    `gorm:"not null;size:3"`
	EncryptedAuthorization []byte    `gorm:"type:bytea;not null"`
	WrappedDEK             []byte    `gorm:"type:bytea;not null"`
	CardType               string    `gorm:"not null;default:'';size:32"`
	Last4                  string    `gorm:"not null;default:'';size:4"`
	ExpMonth               string    `gorm:"not null;default:'';size:2"`
	ExpYear                string    `gorm:"not null;default:'';size:4"`
	Bank                   string    `gorm:"not null;default:'';size:128"`
	CountryCode            string    `gorm:"not null;default:'';size:2"`
	CreatedAt              time.Time
	UpdatedAt              time.Time
}

func (BillingPaymentMethod) TableName() string { return "billing_payment_methods" }
