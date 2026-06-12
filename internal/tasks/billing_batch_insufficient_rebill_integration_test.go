package tasks_test

import (
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

// A row that fails with insufficient_credits must NOT be written off: it stays in
// the unbilled queue (billed_at NULL) so once the org tops up the next batch run
// debits it, rather than losing the spend forever.
func TestBatch_InsufficientThenTopupRebills(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10) // far less than one gen costs

	genID := insertGeneration(t, db, orgID, credID, defaultGenOpts())

	runBatch(t, db)

	g := loadGen(t, db, genID)
	if g.BilledAt != nil {
		t.Fatal("insufficient row must remain unbilled (billed_at NULL) so a top-up can rebill it")
	}
	if g.BillingError != "insufficient_credits" {
		t.Fatalf("billing_error = %q, want insufficient_credits", g.BillingError)
	}
	if g.BillingAttempts != 1 {
		t.Fatalf("billing_attempts = %d, want 1", g.BillingAttempts)
	}

	balBefore, _ := billing.NewCreditsService(db).Balance(orgID)
	if balBefore != 10 {
		t.Fatalf("balance should be untouched while insufficient: %d", balBefore)
	}

	if err := billing.NewCreditsService(db).Grant(orgID, 10_000, billing.ReasonTopup, "topup", "tx-1", nil); err != nil {
		t.Fatalf("topup grant: %v", err)
	}

	// Second sweep: the previously-insufficient row now gets billed.
	runBatch(t, db)

	g2 := loadGen(t, db, genID)
	if g2.BilledAt == nil {
		t.Fatal("after top-up the row should be billed")
	}
	if g2.BillingError != "" {
		t.Errorf("after rebill billing_error should clear, got %q", g2.BillingError)
	}
	if g2.CreditsDebited != creditsForTestCost(1) {
		t.Errorf("credits_debited = %d, want %d after rebill", g2.CreditsDebited, creditsForTestCost(1))
	}

	balAfter, _ := billing.NewCreditsService(db).Balance(orgID)
	wantAfter := int64(10 + 10_000 - creditsForTestCost(1))
	if balAfter != wantAfter {
		t.Errorf("balance after rebill = %d, want %d", balAfter, wantAfter)
	}
}

// A permanently underfunded row must not hot-loop forever: after the retry cap
// it is stamped billed_at (credits_debited=0) and leaves the queue.
func TestBatch_InsufficientHotLoopCapped(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10) // never enough

	genID := insertGeneration(t, db, orgID, credID, defaultGenOpts())

	// Run enough times to exhaust the (small) attempt cap.
	var g genFixture
	for i := 0; i < 10; i++ {
		runBatch(t, db)
		g = loadGen(t, db, genID)
		if g.BilledAt != nil {
			break
		}
	}

	if g.BilledAt == nil {
		t.Fatal("underfunded row should eventually be written off (billed_at set) once retries are exhausted")
	}
	if g.BillingError != "insufficient_credits" {
		t.Errorf("written-off row should keep insufficient_credits error, got %q", g.BillingError)
	}
	if g.CreditsDebited != 0 {
		t.Errorf("written-off row should have credits_debited 0, got %d", g.CreditsDebited)
	}

	// Written-off row leaves the queue; its attempt counter must stop climbing.
	attemptsAtWriteOff := g.BillingAttempts
	runBatch(t, db)
	if after := loadGen(t, db, genID).BillingAttempts; after != attemptsAtWriteOff {
		t.Errorf("billing_attempts kept climbing after write-off: %d -> %d (still hot-looping)", attemptsAtWriteOff, after)
	}
}
