package middleware_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/auth"
	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/model"
)

// TestRequireOrgOwner exercises the owner-only gate: only role=="owner" passes;
// admins and members are rejected 403; unauthenticated is 401.
func TestRequireOrgOwner(t *testing.T) {
	db := connectTestDB(t)

	org := model.Org{ID: uuid.New(), Name: "own-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	owner := model.User{ID: uuid.New(), Email: "own-" + uuid.NewString()[:8] + "@t.com", Name: "owner"}
	admin := model.User{ID: uuid.New(), Email: "adm-" + uuid.NewString()[:8] + "@t.com", Name: "admin"}
	member := model.User{ID: uuid.New(), Email: "mem-" + uuid.NewString()[:8] + "@t.com", Name: "member"}
	rows := []any{
		&org, &owner, &admin, &member,
		&model.OrgMembership{UserID: owner.ID, OrgID: org.ID, Role: "owner"},
		&model.OrgMembership{UserID: admin.ID, OrgID: org.ID, Role: "admin"},
		&model.OrgMembership{UserID: member.ID, OrgID: org.ID, Role: "member"},
	}
	for _, r := range rows {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id IN ?", []uuid.UUID{owner.ID, admin.ID, member.ID}).Delete(&model.User{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})

	mw := middleware.RequireOrgOwner(db)
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })

	call := func(userID *uuid.UUID, withClaims bool) int {
		req := httptest.NewRequest(http.MethodPost, "/", nil)
		req = middleware.WithOrg(req, &org)
		if withClaims && userID != nil {
			req = middleware.WithAuthClaims(req, &auth.AuthClaims{UserID: userID.String(), OrgID: org.ID.String()})
		}
		rr := httptest.NewRecorder()
		mw(next).ServeHTTP(rr, req)
		return rr.Code
	}

	if code := call(&owner.ID, true); code != http.StatusOK {
		t.Fatalf("owner should pass, got %d", code)
	}
	if code := call(&admin.ID, true); code != http.StatusForbidden {
		t.Fatalf("admin should be 403, got %d", code)
	}
	if code := call(&member.ID, true); code != http.StatusForbidden {
		t.Fatalf("member should be 403, got %d", code)
	}
	if code := call(nil, false); code != http.StatusUnauthorized {
		t.Fatalf("no claims should be 401, got %d", code)
	}
	// API-key callers carry no AuthClaims, so the owner-only gate rejects them
	// (billing money-moves are JWT-owner-only, never key-reachable).
	reqKey := httptest.NewRequest(http.MethodPost, "/", nil)
	reqKey = middleware.WithOrg(reqKey, &org)
	reqKey = middleware.WithAPIKeyClaims(reqKey, &middleware.APIKeyClaims{OrgID: org.ID.String()})
	rrKey := httptest.NewRecorder()
	mw(next).ServeHTTP(rrKey, reqKey)
	if rrKey.Code != http.StatusUnauthorized {
		t.Fatalf("api-key should be 401 on owner-only gate, got %d", rrKey.Code)
	}
}
