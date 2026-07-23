package tasks_test

import (
	"testing"

	"github.com/usehivy/hivy/internal/billing"
)

func TestBatch_DebitsXiaomiMiMoFromCapturedTokenUsage(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)

	opts := defaultGenOpts()
	opts.ProviderID = "xiaomi"
	opts.Model = "mimo-v2.5-pro"
	opts.Cost = 0
	opts.CostSource = ""
	opts.InputTokens = 2_000_000
	opts.CachedTokens = 1_000_000
	opts.OutputTokens = 1_000_000
	id := insertGeneration(t, db, orgID, credID, opts)

	runBatch(t, db)

	g := loadGen(t, db, id)
	if g.BillingError != "" {
		t.Fatalf("MiMo generation billing error = %q", g.BillingError)
	}
	if g.BillingCostSource != billing.CostSourceRegistry {
		t.Fatalf("billing_cost_source = %q, want %q", g.BillingCostSource, billing.CostSourceRegistry)
	}

	// 1M uncached input + 1M cached input + 1M output:
	// $0.435 + $0.0036 + $0.87 = $1.3086 = 1,309 credits.
	const wantCredits int64 = 1_309
	if g.CreditsDebited != wantCredits {
		t.Fatalf("credits_debited = %d, want %d", g.CreditsDebited, wantCredits)
	}
	bal, err := billing.NewCreditsService(db).Balance(orgID)
	if err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if bal != 10_000-wantCredits {
		t.Fatalf("balance = %d, want %d", bal, 10_000-wantCredits)
	}
}

func TestBatch_DebitsAtlasCloudHy3FromCapturedTokenUsage(t *testing.T) {
	db := connectDB(t)
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 10_000)

	opts := defaultGenOpts()
	opts.ProviderID = "atlascloud"
	opts.Model = "hy3"
	opts.Cost = 0
	opts.CostSource = ""
	opts.InputTokens = 2_000_000
	opts.CachedTokens = 1_000_000
	opts.OutputTokens = 1_000_000
	id := insertGeneration(t, db, orgID, credID, opts)

	runBatch(t, db)

	g := loadGen(t, db, id)
	if g.BillingError != "" {
		t.Fatalf("Atlas Cloud generation billing error = %q", g.BillingError)
	}
	if g.BillingCostSource != billing.CostSourceRegistry {
		t.Fatalf("billing_cost_source = %q, want %q", g.BillingCostSource, billing.CostSourceRegistry)
	}

	// 1M fresh input at $0.20/M + 1M cached input at the billing-ledger-
	// verified $0.05/M + 1M output at $0.80/M = $1.05.
	const wantCredits int64 = 1_050
	if g.CreditsDebited != wantCredits {
		t.Fatalf("credits_debited = %d, want %d", g.CreditsDebited, wantCredits)
	}
	bal, err := billing.NewCreditsService(db).Balance(orgID)
	if err != nil {
		t.Fatalf("load balance: %v", err)
	}
	if bal != 10_000-wantCredits {
		t.Fatalf("balance = %d, want %d", bal, 10_000-wantCredits)
	}
}
