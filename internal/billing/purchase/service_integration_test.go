package purchase_test

import (
	"context"
	"encoding/base64"
	"errors"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	billingfake "github.com/usehivy/hivy/internal/billing/fake"
	"github.com/usehivy/hivy/internal/billing/purchase"
	"github.com/usehivy/hivy/internal/crypto"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/testdb"
)

func TestIntegration_PurchaseStoresAndReusesPaystackAuthorization(t *testing.T) {
	db, cleanup := connectPurchaseTestDB(t)
	defer cleanup()
	ctx := context.Background()
	org := model.Org{ID: uuid.New(), Name: "purchase-" + uuid.NewString(), Active: true, BillingCurrency: "USD"}
	user := model.User{ID: uuid.New(), Email: "purchase-" + uuid.NewString() + "@example.com"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		_ = db.Unscoped().Delete(&org).Error
		_ = db.Unscoped().Delete(&user).Error
	})

	kms, err := crypto.NewAEADWrapper(ctx, base64.StdEncoding.EncodeToString(make([]byte, 32)), "billing-test")
	if err != nil {
		t.Fatalf("create KMS: %v", err)
	}
	provider := billingfake.New("paystack")
	registry := billing.NewRegistry()
	registry.Register(provider)
	credits := billing.NewCreditsService(db)
	service := purchase.NewService(db, registry, credits, 160_000, kms)

	first, err := service.Create(ctx, purchase.CreateInput{
		OrgID: org.ID, UserID: user.ID, Email: user.Email, PackID: "usd_10",
		IdempotencyKey: uuid.NewString(), SavePaymentMethod: true,
	})
	if err != nil {
		t.Fatalf("create first purchase: %v", err)
	}
	paidAt := time.Now().UTC()
	provider.NextResolveResult = &billing.DepositResult{
		Status: billing.PaymentPaid, PaidAmountMinor: first.Purchase.TotalMinor,
		Currency: billing.CurrencyUSD, PaidAt: &paidAt, CustomerEmail: user.Email,
		Authorization: &billing.PaymentAuthorization{
			AuthorizationCode: "AUTH_scrubbed", CardType: "visa", Last4: "4081",
			ExpMonth: "12", ExpYear: "2030", Bank: "TEST BANK", Channel: "card",
			Signature: "SIG_scrubbed", Reusable: true, CountryCode: "NG",
		},
	}
	if _, err := service.Verify(ctx, org.ID, first.Purchase.ID); err != nil {
		t.Fatalf("verify first purchase: %v", err)
	}
	methods, err := service.ListPaymentMethods(ctx, org.ID, user.ID)
	if err != nil || len(methods) != 1 {
		t.Fatalf("payment methods = %#v, err = %v", methods, err)
	}

	second, err := service.Create(ctx, purchase.CreateInput{
		OrgID: org.ID, UserID: user.ID, Email: "changed@example.com", PackID: "usd_25",
		IdempotencyKey: uuid.NewString(), PaymentMethodID: &methods[0].ID,
	})
	if err != nil {
		t.Fatalf("create saved-card purchase: %v", err)
	}
	charges := provider.SavedCharges()
	if len(charges) != 1 || charges[0].CustomerEmail != user.Email || charges[0].AuthorizationCode != "AUTH_scrubbed" {
		t.Fatalf("saved charges = %#v", charges)
	}
	provider.NextResolveResult = &billing.DepositResult{
		Status: billing.PaymentPaid, PaidAmountMinor: second.Purchase.TotalMinor,
		Currency: billing.CurrencyUSD, PaidAt: &paidAt,
	}
	if _, err := service.Verify(ctx, org.ID, second.Purchase.ID); err != nil {
		t.Fatalf("verify saved-card purchase: %v", err)
	}
	balance, err := credits.Balance(org.ID)
	if err != nil || balance != 35_000 {
		t.Fatalf("balance = %d, err = %v", balance, err)
	}
	if err := service.DeletePaymentMethod(ctx, org.ID, uuid.New(), methods[0].ID); !errors.Is(err, purchase.ErrPaymentMethodNotFound) {
		t.Fatalf("cross-user delete error = %v", err)
	}
	if err := service.DeletePaymentMethod(ctx, org.ID, user.ID, methods[0].ID); err != nil {
		t.Fatalf("delete payment method: %v", err)
	}
	methods, err = service.ListPaymentMethods(ctx, org.ID, user.ID)
	if err != nil || len(methods) != 0 {
		t.Fatalf("payment methods after delete = %#v, err = %v", methods, err)
	}
}

func connectPurchaseTestDB(t *testing.T) (*gorm.DB, func()) {
	t.Helper()
	dsn := testdb.DatabaseURL("DATABASE_URL", "HIVY_DATABASE_URL", "TEST_DATABASE_URL")
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatalf("connect Postgres (run `make test-setup`): %v", err)
	}
	sqlDB, err := db.DB()
	if err != nil {
		t.Fatalf("get SQL DB: %v", err)
	}
	testdb.ApplyMigrations(t, db)
	return db, func() { _ = sqlDB.Close() }
}
