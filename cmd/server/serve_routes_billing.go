package main

import (
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

func mountBillingRoutes(r chi.Router, db *gorm.DB, billingHandler *handler.BillingHandler) {
	if billingHandler == nil {
		return
	}
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireOrgAdmin(db))
		r.Get("/billing/account", billingHandler.GetAccount)
		r.Get("/billing/purchases", billingHandler.ListPurchases)
	})
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireOrgOwner(db))
		r.Put("/billing/account/currency", billingHandler.SelectCurrency)
		r.Post("/billing/purchases", billingHandler.CreatePurchase)
		r.Post("/billing/purchases/{id}/verify", billingHandler.VerifyPurchase)
	})
}
