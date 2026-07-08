package main

import (
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

// mountBillingRoutes registers /v1/billing/* under the JWT-protected router.
// Kept in its own file so serve_routes_v1.go stays under the file-length cap.
//
// Authorization tiers:
//   - /billing/verify — the final hop of a checkout the initiating user already
//     started; reachable by any authenticated org member, but org-scoped inside
//     the handler (ResolveCheckout asserts ExpectedOrgID), so it cannot touch
//     another org and cannot provision without a real paid reference.
//   - GET /billing/subscription + preview-change — admin+ reads (card detail is
//     additionally stripped for non-owners inside GetSubscription).
//   - every money-moving mutation — owner-only (RequireOrgOwner).
func mountBillingRoutes(r chi.Router, db *gorm.DB, billingHandler *handler.BillingHandler, subscriptionHandler *handler.SubscriptionHandler) {
	if billingHandler == nil {
		return
	}
	r.Post("/billing/verify", billingHandler.Verify)

	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireOrgAdmin(db))
		r.Get("/billing/subscription", billingHandler.GetSubscription)
		r.Post("/billing/subscription/preview-change", subscriptionHandler.PreviewChange)
	})

	// Money moves are owner-only: a non-owner admin must not be able to buy,
	// upgrade/downgrade, cancel, or resume a paid plan.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireOrgOwner(db))
		r.Post("/billing/checkout", billingHandler.CreateCheckout)
		r.Post("/billing/subscription/init-upgrade", subscriptionHandler.InitUpgrade)
		r.Post("/billing/subscription/apply-change", subscriptionHandler.ApplyChange)
		r.Post("/billing/subscription/cancel", subscriptionHandler.Cancel)
		r.Post("/billing/subscription/resume", subscriptionHandler.Resume)
	})
}
