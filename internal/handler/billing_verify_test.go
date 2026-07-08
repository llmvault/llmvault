package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

func TestBillingVerify_CreatesSubscriptionWithMatchingAmount(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-pro-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:             billing.StatusActive,
		PaidAmountMinor:    plan.PriceCents,
		Currency:           plan.Currency,
		Reference:          "ref_ok",
		ExternalCustomerID: "CUS_ok",
		PaidAt:             paidAt(),
		Metadata:           map[string]string{"plan_slug": plan.Slug, "org_id": org.ID.String()},
		PaymentMethod: billing.PaymentMethod{
			AuthorizationCode: "AUTH_ok",
			Channel:           billing.ChannelCard,
			CardLast4:         "4242",
		},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_ok"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "active" {
		t.Errorf("status = %q, want active", resp["status"])
	}
	if resp["plan_slug"] != plan.Slug {
		t.Errorf("plan_slug = %q, want %q", resp["plan_slug"], plan.Slug)
	}

	var sub model.Subscription
	if err := h.db.Where("org_id = ? AND plan_id = ?", org.ID, plan.ID).First(&sub).Error; err != nil {
		t.Fatalf("expected subscription, got: %v", err)
	}
	if sub.AuthorizationCode != "AUTH_ok" {
		t.Errorf("AuthorizationCode = %q", sub.AuthorizationCode)
	}
	if sub.PaymentChannel != "card" {
		t.Errorf("PaymentChannel = %q, want card", sub.PaymentChannel)
	}
	if sub.CardLast4 != "4242" {
		t.Errorf("CardLast4 = %q", sub.CardLast4)
	}
}

func TestBillingVerify_AmountMismatchReturns402(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-mismatch-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: plan.PriceCents - 1,
		Currency:        plan.Currency,
		Reference:       "ref_short",
		PaidAt:          paidAt(),
		Metadata:        map[string]string{"plan_slug": plan.Slug, "org_id": org.ID.String()},
		PaymentMethod:   billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_short"})
	if rr.Code != http.StatusPaymentRequired {
		t.Fatalf("expected 402, got %d: %s", rr.Code, rr.Body.String())
	}

	// Subscription not created.
	var count int64
	h.db.Model(&model.Subscription{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 subscriptions on amount mismatch, got %d", count)
	}
}

func TestBillingVerify_UnsupportedChannelReturns400(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-channel-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: plan.PriceCents,
		Currency:        plan.Currency,
		Reference:       "ref_ussd",
		PaidAt:          paidAt(),
		Metadata:        map[string]string{"plan_slug": plan.Slug, "org_id": org.ID.String()},
		PaymentMethod:   billing.PaymentMethod{Channel: billing.PaymentChannel("ussd"), AuthorizationCode: "AUTH"},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_ussd"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBillingVerify_NoMetadataReturns400(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	h.seedPlan(t, "no-meta-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: 2_000_000,
		Currency:        "NGN",
		Reference:       "ref_no_meta",
		// org_id present so the org-match guard passes; plan_slug absent so we
		// still exercise the missing-plan_slug → 400 path.
		Metadata:      map[string]string{"org_id": org.ID.String()},
		PaymentMethod: billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_no_meta"})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBillingVerify_NonSuccessReturnsStatus(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:   billing.StatusPastDue,
		Metadata: map[string]string{"org_id": org.ID.String()},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_pending"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rr.Code, rr.Body.String())
	}
	var resp map[string]string
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if resp["status"] != "past_due" {
		t.Errorf("status = %q, want past_due", resp["status"])
	}

	var count int64
	h.db.Model(&model.Subscription{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 subscriptions, got %d", count)
	}
}

// TestBillingVerify_CrossOrgReferenceRejected asserts that a valid, paid
// reference initialised for a DIFFERENT org cannot be replayed to flip this
// org's plan — the ResolveCheckout org-mismatch guard must reject it, no
// subscription created.
func TestBillingVerify_CrossOrgReferenceRejected(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-xorg-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	otherOrg := uuid.New()
	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:          billing.StatusActive,
		PaidAmountMinor: plan.PriceCents,
		Currency:        plan.Currency,
		Reference:       "ref_other_org",
		PaidAt:          paidAt(),
		Metadata:        map[string]string{"plan_slug": plan.Slug, "org_id": otherOrg.String()},
		PaymentMethod:   billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_other_org"})
	if rr.Code == http.StatusOK {
		t.Fatalf("cross-org reference must be rejected, got 200: %s", rr.Body.String())
	}

	var count int64
	h.db.Model(&model.Subscription{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 0 {
		t.Errorf("expected 0 subscriptions on cross-org reference, got %d", count)
	}
}

func TestBillingVerify_RequiresReference(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)

	rr := h.post(t, user.ID, org.ID, map[string]string{})
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rr.Code, rr.Body.String())
	}
}

func TestBillingVerify_GrantsMonthlyCredits(t *testing.T) {
	h := newVerifyHarness(t)
	org, user := h.seedOrgWithMember(t)
	plan := h.seedPlan(t, "verify-credits-"+uuid.NewString()[:8], 2_000_000, 5_000, "NGN")

	h.provider.NextResolveResult = &billing.ResolveCheckoutResult{
		Status:             billing.StatusActive,
		PaidAmountMinor:    plan.PriceCents,
		Currency:           plan.Currency,
		Reference:          "ref_grant",
		ExternalCustomerID: "CUS_grant",
		PaidAt:             paidAt(),
		Metadata:           map[string]string{"plan_slug": plan.Slug, "org_id": org.ID.String()},
		PaymentMethod:      billing.PaymentMethod{Channel: billing.ChannelCard, AuthorizationCode: "AUTH"},
	}

	rr := h.post(t, user.ID, org.ID, map[string]string{"reference": "ref_grant"})
	if rr.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rr.Code)
	}

	credits := billing.NewCreditsService(h.db)
	bal, err := credits.Balance(org.ID)
	if err != nil {
		t.Fatalf("balance: %v", err)
	}
	if bal != plan.MonthlyCredits {
		t.Errorf("balance = %d, want %d (monthly credits)", bal, plan.MonthlyCredits)
	}
}
