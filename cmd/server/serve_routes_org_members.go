package main

import (
	"github.com/go-chi/chi/v5"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/handler"
	"github.com/usehivy/hivy/internal/middleware"
)

// mountOrgMemberLifecycleRoutes wires the org-membership lifecycle: team
// provisioning (team connection, skill, and RAG grants) and member role/removal
// are admin-only, while ownership transfer and org deletion are owner-only.
func mountOrgMemberLifecycleRoutes(r chi.Router, database *gorm.DB) {
	orgMemberHandler := handler.NewOrgMemberHandler(database)
	teamProvisioning := handler.NewTeamProvisioningHandler(database)
	const orgCurrentPath = "/orgs/current"

	// Reading a team's connections and skills is a member action, gated in-handler
	// to members of that team; every mutation stays admin-only below.
	teamProvisioning.MountReadable(r, orgCurrentPath)
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireOrgAdmin(database))
		teamProvisioning.Mount(r, orgCurrentPath)
		r.Patch(orgCurrentPath+"/members/{userID}/role", orgMemberHandler.PatchRole)
		r.Delete(orgCurrentPath+"/members/{userID}", orgMemberHandler.Remove)
	})

	// Ownership transfer and org deletion are owner-only.
	r.Group(func(r chi.Router) {
		r.Use(middleware.RequireOrgOwner(database))
		r.Post(orgCurrentPath+"/transfer-ownership", orgMemberHandler.TransferOwnership)
		r.Delete(orgCurrentPath, orgMemberHandler.DeleteOrg)
	})
}
