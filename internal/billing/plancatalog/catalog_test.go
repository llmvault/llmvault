package plancatalog

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadDefaultCatalog(t *testing.T) {
	catalog, err := Load("")
	if err != nil {
		t.Fatalf("load default catalog: %v", err)
	}
	if catalog.Version != 1 {
		t.Fatalf("version got %d, want 1", catalog.Version)
	}
	if len(catalog.Plans) == 0 {
		t.Fatalf("expected at least one plan")
	}

	plansBySlug := make(map[string]PlanSpec, len(catalog.Plans))
	for _, plan := range catalog.Plans {
		plansBySlug[plan.Slug] = plan
	}

	free, ok := plansBySlug["free"]
	if !ok {
		t.Fatalf("catalog should include free plan")
	}
	if free.WelcomeCredits != 1000 {
		t.Fatalf("free welcome credits got %d, want 1000", free.WelcomeCredits)
	}
	if free.Currency != "USD" {
		t.Fatalf("free currency got %q, want USD", free.Currency)
	}

	wantPaid := map[string]struct {
		priceCents     int64
		monthlyCredits int64
	}{
		"business-15000":  {priceCents: 2900, monthlyCredits: 15000},
		"business-20000":  {priceCents: 3900, monthlyCredits: 20000},
		"business-45000":  {priceCents: 7900, monthlyCredits: 45000},
		"business-80000":  {priceCents: 13900, monthlyCredits: 80000},
		"business-180000": {priceCents: 29900, monthlyCredits: 180000},
		"business-300000": {priceCents: 49900, monthlyCredits: 300000},
	}
	for slug, want := range wantPaid {
		plan, ok := plansBySlug[slug]
		if !ok {
			t.Fatalf("catalog missing paid plan %q", slug)
		}
		if !boolValue(plan.Visible) || !boolValue(plan.Active) {
			t.Fatalf("paid plan %q should be visible and active", slug)
		}
		if plan.Provider != "paystack" {
			t.Fatalf("paid plan %q provider got %q, want paystack", slug, plan.Provider)
		}
		if plan.Currency != "USD" {
			t.Fatalf("paid plan %q currency got %q, want USD", slug, plan.Currency)
		}
		if plan.PriceCents != want.priceCents {
			t.Fatalf("paid plan %q price got %d, want %d", slug, plan.PriceCents, want.priceCents)
		}
		if plan.MonthlyCredits != want.monthlyCredits {
			t.Fatalf("paid plan %q credits got %d, want %d", slug, plan.MonthlyCredits, want.monthlyCredits)
		}
		if !plan.Paystack.Sync || plan.Paystack.Interval != "monthly" {
			t.Fatalf("paid plan %q should sync monthly to paystack metadata", slug)
		}
	}

	for _, slug := range []string{
		"business-2500",
		"business-10000",
		"business-25000",
		"business-100000",
	} {
		plan, ok := plansBySlug[slug]
		if !ok {
			t.Fatalf("catalog missing retired plan %q", slug)
		}
		if boolValue(plan.Visible) || boolValue(plan.Active) {
			t.Fatalf("retired plan %q should be hidden and inactive", slug)
		}
		if plan.Paystack.Sync {
			t.Fatalf("retired plan %q should not sync to paystack", slug)
		}
	}
}

func boolValue(v *bool) bool {
	return v != nil && *v
}

func TestLoadRejectsDuplicateSlugs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "catalog.json")
	body := `{
		"version": 1,
		"plans": [
			{"slug":"free","name":"Free","provider":"","visible":true,"active":true,"monthly_credits":0,"welcome_credits":500,"price_cents":0,"currency":"NGN","features":[]},
			{"slug":"free","name":"Free Copy","provider":"","visible":true,"active":true,"monthly_credits":0,"welcome_credits":500,"price_cents":0,"currency":"NGN","features":[]}
		]
	}`
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write catalog: %v", err)
	}
	_, err := Load(path)
	if err == nil || !strings.Contains(err.Error(), `duplicate plan slug "free"`) {
		t.Fatalf("error got %v, want duplicate slug", err)
	}
}
