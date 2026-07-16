package billing_test

import (
	"errors"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

// Two concurrent GrantWithTx calls with the same idempotency tuple
// (org_id, reason, ref_type, ref_id) must result in exactly one ledger entry.
// Without the partial unique index both goroutines pass the check-then-insert and
// the org receives free credits.
func TestIntegration_Credits_ConcurrentGrantSameRef_NoDoubleGrant(t *testing.T) {
	db := connectCreditsTestDB(t)
	svc := billing.NewCreditsService(db)
	orgID := seedOrg(t, db)

	const (
		amount = 1000
		goros  = 8
	)

	var wg sync.WaitGroup
	results := make([]error, goros)
	start := make(chan struct{})
	for i := 0; i < goros; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			<-start
			results[idx] = svc.Grant(orgID, amount, billing.ReasonTopup, "credit_purchase", "ref-race-1")
		}(i)
	}
	close(start)
	wg.Wait()

	var successes, alreadyRecorded, other int
	for _, err := range results {
		switch {
		case err == nil:
			successes++
		case errors.Is(err, billing.ErrAlreadyRecorded):
			alreadyRecorded++
		default:
			other++
			t.Errorf("unexpected grant error: %v", err)
		}
	}
	if other != 0 {
		t.Fatalf("had %d unexpected errors", other)
	}
	// At most one goroutine inserts; the rest observe the idempotent replay.
	if successes > 1 {
		t.Errorf("got %d successful grants, want at most 1 (double-grant race)", successes)
	}

	bal, err := svc.Balance(orgID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != amount {
		t.Errorf("balance after %d concurrent same-ref grants = %d, want %d (no double credit)", goros, bal, amount)
	}
}
