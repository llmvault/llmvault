package main

import (
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

// mountOrgMemberLifecycleRoutes wires the org-membership lifecycle: team
// provisioning (team plugin allowlist + team RAG grants) and member role/removal
// are admin-only, while ownership transfer and org deletion are owner-only.
func mountOrgMemberLifecycleRoutes(r chi.Router, database *gorm.DB) {
	orgMemberHandler := handler.NewOrgMemberHandler(database)
	teamProvisioning := handler.NewTeamProvisioningHandler(database)
	r.Route("/orgs/current", func(r chi.Router) {
		// Reading a team's enabled plugins is a member action, gated in-handler
		// to members of that team; every mutation stays admin-only below.
		teamProvisioning.MountReadable(r)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(database))
			teamProvisioning.Mount(r)
			r.Patch("/members/{userID}/role", orgMemberHandler.PatchRole)
			r.Delete("/members/{userID}", orgMemberHandler.Remove)
		})
	})
	// Ownership transfer and org deletion are owner-only.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireOrgOwner(database))
		r.Post("/orgs/current/transfer-ownership", orgMemberHandler.TransferOwnership)
		r.Delete("/orgs/current", orgMemberHandler.DeleteOrg)
	})
}
