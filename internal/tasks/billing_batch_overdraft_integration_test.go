package tasks_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/middleware"
)

// bigGenOpts returns a single generation whose provider cost maps to exactly
// `credits` credits, so a test can drive a large per-org delta with one row.
func bigGenOpts(credits int64) genOpts {
	opts := defaultGenOpts()
	opts.Cost = float64(credits) * billing.CreditUSDValue
	return opts
}

// gateBlocks reports whether RequireCredits 402s a platform-key request for the
// org, exercising the real admission gate against the live ledger balance.
func gateBlocks(t *testing.T, db *gorm.DB, orgID uuid.UUID) bool {
	t.Helper()
	h := middleware.RequireCredits(billing.NewCreditsService(db))(http.HandlerFunc(
		func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) },
	))
	r := middleware.WithClaims(
		httptest.NewRequest(http.MethodPost, "/v1/proxy/chat/completions", nil),
		&middleware.TokenClaims{OrgID: orgID.String(), IsSystem: true},
	)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w.Code == http.StatusPaymentRequired
}

// (a) An org that outspends its balance is debited down to the floor — not
// refused outright — and the admission gate then 402s.
func TestBatch_OverspendDebitsDownToFloorThenGates(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 400)

	genID := insertGeneration(t, db, orgID, credID, bigGenOpts(600))
	runBatch(t, db)

	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	if bal != billing.CreditOverdraftFloor {
		t.Fatalf("balance = %d, want floor %d (450 of 600 debited)", bal, billing.CreditOverdraftFloor)
	}

	g := loadGen(t, db, genID)
	if g.BilledAt == nil {
		t.Error("the partially-paid row should be billed (credits were taken)")
	}
	if g.CreditsDebited != 450 {
		t.Errorf("credits_debited = %d, want 450 (400 balance + 50 overdraft)", g.CreditsDebited)
	}

	if !gateBlocks(t, db, orgID) {
		t.Error("require_credits should 402 once the balance is at the floor")
	}
}

// (b) An org starting at zero can still overspend through the overdraft window
// down to the floor, and no further batch drives it below.
func TestBatch_ZeroBalanceOverspendsToFloorThenCapped(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 0)

	insertGeneration(t, db, orgID, credID, bigGenOpts(600))
	runBatch(t, db)

	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	if bal != billing.CreditOverdraftFloor {
		t.Fatalf("balance = %d, want floor %d (only the 50 overdraft is debited)", bal, billing.CreditOverdraftFloor)
	}
	if !gateBlocks(t, db, orgID) {
		t.Error("require_credits should 402 once the balance is at the floor")
	}

	// A further generation must not push the balance below the floor.
	insertGeneration(t, db, orgID, credID, bigGenOpts(600))
	runBatch(t, db)

	bal2, _ := billing.NewCreditsService(db).Balance(orgID)
	if bal2 != billing.CreditOverdraftFloor {
		t.Fatalf("balance = %d, want it capped at floor %d", bal2, billing.CreditOverdraftFloor)
	}
}

// (c) When the balance comfortably covers the delta, the floor logic is inert:
// the full delta is debited and no row is flagged insufficient.
func TestBatch_SufficientBalanceDebitsFullDelta(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)

	genID := insertGeneration(t, db, orgID, credID, bigGenOpts(600))
	runBatch(t, db)

	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	if bal != 10_000-600 {
		t.Fatalf("balance = %d, want %d (full delta debited)", bal, 10_000-600)
	}

	g := loadGen(t, db, genID)
	if g.BillingError != "" {
		t.Errorf("well-funded row should not be flagged, got %q", g.BillingError)
	}
	if g.CreditsDebited != 600 {
		t.Errorf("credits_debited = %d, want 600", g.CreditsDebited)
	}
	if gateBlocks(t, db, orgID) {
		t.Error("well-funded org should not be gated")
	}
}
