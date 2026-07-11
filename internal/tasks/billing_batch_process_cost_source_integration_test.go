package tasks_test

import (
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

func TestBatch_PreservesPrefilledRegistryCostSource(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)
	opts := defaultGenOpts()
	opts.CostSource = billing.CostSourceRegistry

	id := insertGeneration(t, db, orgID, credID, opts)
	runBatch(t, db)

	if got := loadGen(t, db, id).BillingCostSource; got != billing.CostSourceRegistry {
		t.Fatalf("billing_cost_source = %q, want %q", got, billing.CostSourceRegistry)
	}
}
