package handler_test

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/billing/fake"
	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

type verifyHarness struct {
	db       *gorm.DB
	router   *chi.Mux
	provider *fake.Provider
}

func newVerifyHarness(t *testing.T) *verifyHarness {
	t.Helper()
	db := connectTestDB(t)
	registry := billing.NewRegistry()
	provider := fake.New("paystack")
	registry.Register(provider)
	billingHandler := handler.NewBillingHandler(db, registry, billing.NewCreditsService(db))

	r := chi.NewRouter()
	r.Route("/v1/billing", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Post("/verify", billingHandler.Verify)
	})
	return &verifyHarness{db: db, router: r, provider: provider}
}

func (h *verifyHarness) seedOrgWithMember(t *testing.T) (model.Org, model.User) {
	t.Helper()
	user := model.User{Email: "verify-" + uuid.NewString()[:8] + "@test.com", Name: "Verify"}
	if err := h.db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "Verify Org " + uuid.NewString()[:8], Active: true}
	if err := h.db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	if err := h.db.Create(&model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: "owner"}).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("org_id = ?", org.ID).Delete(&model.Subscription{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		h.db.Where("org_id = ?", org.ID).Delete(&model.CreditLedgerEntry{})
		h.db.Where("id = ?", org.ID).Delete(&model.Org{})
		h.db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return org, user
}

func (h *verifyHarness) seedPlan(t *testing.T, slug string, priceCents int64, monthlyCredits int64, currency string) model.Plan {
	t.Helper()
	plan := model.Plan{
		ID:             uuid.New(),
		Slug:           slug,
		Name:           "Plan " + slug,
		PriceCents:     priceCents,
		Currency:       currency,
		MonthlyCredits: monthlyCredits,
		Active:         true,
	}
	if err := h.db.Create(&plan).Error; err != nil {
		t.Fatalf("create plan: %v", err)
	}
	t.Cleanup(func() {
		h.db.Where("plan_id = ?", plan.ID).Delete(&model.Subscription{})
		h.db.Where("id = ?", plan.ID).Delete(&model.Plan{})
	})
	return plan
}

func (h *verifyHarness) post(t *testing.T, userID, orgID uuid.UUID, body any) *httptest.ResponseRecorder {
	t.Helper()
	buf := new(bytes.Buffer)
	_ = json.NewEncoder(buf).Encode(body)
	req := httptest.NewRequest("POST", "/v1/billing/verify", buf)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Org-ID", orgID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{
		UserID: userID.String(),
		OrgID:  orgID.String(),
		Role:   "owner",
	})
	rr := httptest.NewRecorder()
	h.router.ServeHTTP(rr, req)
	return rr
}

func paidAt() *time.Time {
	t := time.Now().UTC().Truncate(time.Second)
	return &t
}
