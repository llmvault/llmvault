package handler_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// These pin route-level authorization: a non-admin org member may read but not
// mutate credentials/tokens or RAG sources, while an admin can do both. They
// mirror the middleware chains in cmd/server/serve_routes_v1.go.

func seedMember(t *testing.T, db *gorm.DB, role string) (model.Org, model.User) {
	t.Helper()
	user := model.User{Email: "gating-" + uuid.NewString() + "@test.com", Name: "Gating"}
	if err := db.Create(&user).Error; err != nil {
		t.Fatalf("create user: %v", err)
	}
	org := model.Org{Name: "Org-" + uuid.NewString()[:8], Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	m := model.OrgMembership{UserID: user.ID, OrgID: org.ID, Role: role}
	if err := db.Create(&m).Error; err != nil {
		t.Fatalf("create membership: %v", err)
	}
	t.Cleanup(func() {
		db.Where("user_id = ?", user.ID).Delete(&model.OrgMembership{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
		db.Where("id = ?", user.ID).Delete(&model.User{})
	})
	return org, user
}

func okHandler(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }

func doJWT(t *testing.T, router http.Handler, method, path string, userID, orgID uuid.UUID) int {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	req.Header.Set("X-Org-ID", orgID.String())
	req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: userID.String(), OrgID: orgID.String()})
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	return rr.Code
}

// credentials/tokens mutations are admin-gated for JWT callers; reads stay open.
func TestRouteGating_CredentialsTokensMutationsAdminOnly(t *testing.T) {
	db := connectTestDB(t)

	router := chi.NewRouter()
	router.Route("/v1", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Group(func(r chi.Router) {
			r.Get("/credentials", okHandler)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdminOrAPIKey(db))
				r.Post("/credentials", okHandler)
				r.Delete("/credentials/{id}", okHandler)
			})
		})
		r.Group(func(r chi.Router) {
			r.Get("/tokens", okHandler)
			r.Group(func(r chi.Router) {
				r.Use(middleware.RequireOrgAdminOrAPIKey(db))
				r.Post("/tokens", okHandler)
				r.Delete("/tokens/{jti}", okHandler)
			})
		})
	})

	adminOrg, admin := seedMember(t, db, "admin")
	memberOrg, member := seedMember(t, db, "member")

	if code := doJWT(t, router, http.MethodGet, "/v1/credentials", member.ID, memberOrg.ID); code != http.StatusOK {
		t.Fatalf("member GET /credentials = %d, want 200", code)
	}
	if code := doJWT(t, router, http.MethodPost, "/v1/credentials", member.ID, memberOrg.ID); code != http.StatusForbidden {
		t.Fatalf("member POST /credentials = %d, want 403 (escalation)", code)
	}
	if code := doJWT(t, router, http.MethodPost, "/v1/tokens", member.ID, memberOrg.ID); code != http.StatusForbidden {
		t.Fatalf("member POST /tokens = %d, want 403 (escalation)", code)
	}

	if code := doJWT(t, router, http.MethodPost, "/v1/credentials", admin.ID, adminOrg.ID); code != http.StatusOK {
		t.Fatalf("admin POST /credentials = %d, want 200", code)
	}
	if code := doJWT(t, router, http.MethodPost, "/v1/tokens", admin.ID, adminOrg.ID); code != http.StatusOK {
		t.Fatalf("admin POST /tokens = %d, want 200", code)
	}
}

// RAG source mutations are admin-only; reads stay member-visible.
func TestRouteGating_RAGMutationsAdminOnly(t *testing.T) {
	db := connectTestDB(t)

	router := chi.NewRouter()
	router.Route("/v1/rag", func(r chi.Router) {
		r.Use(middleware.ResolveOrgFromHeader(db))
		r.Get("/sources", okHandler)
		r.Group(func(r chi.Router) {
			r.Use(middleware.RequireOrgAdmin(db))
			r.Post("/sources", okHandler)
			r.Delete("/sources/{id}", okHandler)
			r.Post("/sources/{id}/sync", okHandler)
			r.Post("/sources/{id}/prune", okHandler)
		})
	})

	adminOrg, admin := seedMember(t, db, "owner")
	memberOrg, member := seedMember(t, db, "member")

	if code := doJWT(t, router, http.MethodGet, "/v1/rag/sources", member.ID, memberOrg.ID); code != http.StatusOK {
		t.Fatalf("member GET /rag/sources = %d, want 200", code)
	}
	if code := doJWT(t, router, http.MethodPost, "/v1/rag/sources", member.ID, memberOrg.ID); code != http.StatusForbidden {
		t.Fatalf("member POST /rag/sources = %d, want 403", code)
	}
	if code := doJWT(t, router, http.MethodPost, "/v1/rag/sources/"+uuid.NewString()+"/sync", member.ID, memberOrg.ID); code != http.StatusForbidden {
		t.Fatalf("member POST /rag/sources/{id}/sync = %d, want 403", code)
	}
	if code := doJWT(t, router, http.MethodPost, "/v1/rag/sources", admin.ID, adminOrg.ID); code != http.StatusOK {
		t.Fatalf("admin POST /rag/sources = %d, want 200", code)
	}
}
