package tasks_test

import (
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

// TestBatch_InsufficientThenTopupRebills exercises the P0-8 fix: a row that
// fails with insufficient_credits is NOT written off. It stays in the unbilled
// queue (billed_at NULL), and once the org tops up, the next batch run debits
// it. Before the fix the row was stamped billed_at with credits_debited=0 and
// the spend was lost forever.
func TestBatch_InsufficientThenTopupRebills(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10) // far less than one gen costs

	genID := insertGeneration(t, db, orgID, credID, defaultGenOpts())

	// First sweep: balance is too low, row goes insufficient but stays unbilled.
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

	// Top up the org so it can now afford the generation.
	if err := billing.NewCreditsService(db).Grant(orgID, 10_000, billing.ReasonTopup, "topup", "tx-1", nil); err != nil {
		t.Fatalf("topup grant: %v", err)
	}

	// Second sweep: the previously-insufficient row is still in the queue and
	// now gets billed.
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

// TestBatch_InsufficientHotLoopCapped verifies that a permanently underfunded
// row does not stay in the queue forever: after the retry cap is reached it is
// stamped billed_at (with credits_debited=0) so it cannot hot-loop the batch.
func TestBatch_InsufficientHotLoopCapped(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10) // never enough

	genID := insertGeneration(t, db, orgID, credID, defaultGenOpts())

	// Run the batch enough times to exhaust the attempt cap. The cap is small;
	// 10 runs is comfortably more than enough.
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

	// Once written off the row no longer appears in the unbilled queue, so its
	// attempt counter stops climbing — confirm a further run does not change it.
	attemptsAtWriteOff := g.BillingAttempts
	runBatch(t, db)
	if after := loadGen(t, db, genID).BillingAttempts; after != attemptsAtWriteOff {
		t.Errorf("billing_attempts kept climbing after write-off: %d -> %d (still hot-looping)", attemptsAtWriteOff, after)
	}
}
