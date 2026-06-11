package subscription_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

// The renewal charge must carry a deterministic idempotency reference derived from
// the subscription id and period boundary, so a retry after a post-charge crash
// re-presents the same reference (Paystack dedupes) instead of charging again.
func TestService_Renew_DeterministicChargeReference(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	sub := h.seedRenewSub(t, h.planPro)

	wantRef := fmt.Sprintf("renew-%s-%d", sub.ID, sub.CurrentPeriodEnd.UTC().Unix())

	if _, err := h.service.Renew(context.Background(), sub.ID); err != nil {
		t.Fatalf("Renew: %v", err)
	}

	charges := h.provider.Charges()
	if len(charges) != 1 {
		t.Fatalf("charges = %d, want 1", len(charges))
	}
	if charges[0].Reference != wantRef {
		t.Errorf("charge Reference = %q, want %q", charges[0].Reference, wantRef)
	}
}

// Double-charge window: the charge succeeds at the provider but the DB advance
// fails, so the worker retries. Both attempts must present the SAME deterministic
// reference — that is what makes Paystack dedupe the card charge.
func TestService_Renew_RetryAfterCommitFailureReusesReference(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	sub := h.seedRenewSub(t, h.planPro)

	wantRef := fmt.Sprintf("renew-%s-%d", sub.ID, sub.CurrentPeriodEnd.UTC().Unix())

	if _, err := h.service.Renew(context.Background(), sub.ID); err != nil {
		t.Fatalf("first Renew: %v", err)
	}

	// Reset the period to the past to model "the first advance did not durably
	// commit", forcing a second renewal attempt.
	if err := h.db.Model(&model.Subscription{}).Where("id = ?", sub.ID).
		Updates(map[string]any{
			"current_period_end":   sub.CurrentPeriodEnd,
			"current_period_start": sub.CurrentPeriodStart,
			"renewal_attempts":     0,
			"status":               string(billing.StatusActive),
		}).Error; err != nil {
		t.Fatalf("reset period: %v", err)
	}

	if _, err := h.service.Renew(context.Background(), sub.ID); err != nil {
		t.Fatalf("second Renew: %v", err)
	}

	charges := h.provider.Charges()
	if len(charges) != 2 {
		t.Fatalf("charges = %d, want 2 (one per attempt)", len(charges))
	}
	for i, c := range charges {
		if c.Reference != wantRef {
			t.Errorf("charge[%d] Reference = %q, want %q (must be stable across retries)", i, c.Reference, wantRef)
		}
	}
}

// The inverse: a genuinely new renewal cycle (advanced CurrentPeriodEnd) must get a different
// reference so the next cycle's charge is not dedup-suppressed by Paystack.
func TestService_Renew_DistinctPeriodsGetDistinctReferences(t *testing.T) {
	h := newHarness(t)
	h.seedOwnerEmail(t)
	sub := h.seedRenewSub(t, h.planPro)

	if _, err := h.service.Renew(context.Background(), sub.ID); err != nil {
		t.Fatalf("first Renew: %v", err)
	}
	firstRef := h.provider.Charges()[0].Reference

	// Advance the clock past the new period end so a second renewal becomes due.
	var advanced model.Subscription
	if err := h.db.First(&advanced, "id = ?", sub.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	h.service.SetClock(func() time.Time { return advanced.CurrentPeriodEnd.Add(time.Minute) })

	if _, err := h.service.Renew(context.Background(), sub.ID); err != nil {
		t.Fatalf("second Renew: %v", err)
	}

	charges := h.provider.Charges()
	if len(charges) != 2 {
		t.Fatalf("charges = %d, want 2", len(charges))
	}
	secondRef := charges[1].Reference
	if secondRef == firstRef {
		t.Errorf("distinct renewal cycles produced the same reference %q — next cycle's charge would be dedup-suppressed", firstRef)
	}
}
