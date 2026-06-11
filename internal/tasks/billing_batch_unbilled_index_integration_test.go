package tasks_test

import (
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

// TestBatch_UnbilledIndexExistsAndSupportsScan verifies migration 000033
// (P2-44): the supporting indexes for the billing batch tick are present, the
// partial unbilled index covers exactly the selectUnbilledBatch predicate, and
// the batch still bills correctly with the index in place.
func TestBatch_UnbilledIndexExistsAndSupportsScan(t *testing.T) {
	db := connectDB(t)

	type indexRow struct {
		IndexName string `gorm:"column:indexname"`
		IndexDef  string `gorm:"column:indexdef"`
	}
	wantIndexes := map[string][]string{
		"idx_gen_unbilled_system_created": {"created_at", "billed_at IS NULL", "is_system"},
		"idx_gen_billed_org_cost":         {"org_id", "cost", "billed_at IS NOT NULL", "billing_error", "is_system"},
	}
	for name, wantFragments := range wantIndexes {
		var row indexRow
		if err := db.Raw(
			`SELECT indexname, indexdef FROM pg_indexes WHERE tablename = 'generations' AND indexname = ?`, name,
		).Scan(&row).Error; err != nil {
			t.Fatalf("query pg_indexes for %s: %v", name, err)
		}
		if row.IndexName != name {
			t.Fatalf("expected index %s to exist", name)
		}
		for _, frag := range wantFragments {
			if !strings.Contains(row.IndexDef, frag) {
				t.Fatalf("index %s def %q missing fragment %q", name, row.IndexDef, frag)
			}
		}
	}

	// Functional regression: with the indexes present, the batch still drains the
	// unbilled queue and debits the org exactly once for the batch.
	orgID, credID := seedOrgWithCredentialAndCredits(t, db, 1_000_000)
	const n = 12
	for i := 0; i < n; i++ {
		insertGeneration(t, db, orgID, credID, defaultGenOpts())
	}
	runBatch(t, db)

	var unbilled int64
	db.Model(&model.Generation{}).
		Where("org_id = ? AND billed_at IS NULL AND is_system = TRUE", orgID).
		Count(&unbilled)
	if unbilled != 0 {
		t.Fatalf("%d rows still unbilled after batch", unbilled)
	}
	bal, _ := billing.NewCreditsService(db).Balance(orgID)
	if want := int64(1_000_000 - creditsForTestCost(n)); bal != want {
		t.Fatalf("balance = %d, want %d", bal, want)
	}
}
