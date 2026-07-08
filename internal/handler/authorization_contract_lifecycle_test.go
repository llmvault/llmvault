package handler_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

// seedActiveSubscription attaches an active card subscription to an org so the
// billing payment-detail assertions have card fields to reveal/strip.
func seedActiveSubscription(t *testing.T, db *gorm.DB, orgID uuid.UUID) {
	t.Helper()
	plan := model.Plan{ID: uuid.New(), Slug: "authz-plan-" + uuid.NewString()[:8], Name: "Plan", PriceCents: 1000, Currency: "NGN", Active: true}
	now := time.Now().UTC().Truncate(time.Second)
	sub := model.Subscription{
		OrgID: orgID, PlanID: plan.ID, Provider: "paystack", ExternalCustomerID: "CUS_authz",
		Status: string(billing.StatusActive), CurrentPeriodStart: now.Add(-24 * time.Hour), CurrentPeriodEnd: now.Add(24 * time.Hour),
		PaymentChannel: "card", CardLast4: "4242", CardBrand: "visa", AuthorizationCode: "AUTH_authz",
	}
	for _, r := range []any{&plan, &sub} {
		if err := db.Create(r).Error; err != nil {
			t.Fatalf("seed subscription: %v", err)
		}
	}
	t.Cleanup(func() {
		db.Where("org_id = ?", orgID).Delete(&model.Subscription{})
		db.Where("id = ?", plan.ID).Delete(&model.Plan{})
	})
}

// TestAuthorizationContract_Billing covers matrix area 6: money-moves are
// owner-only and the payment-method snapshot is owner-only. The admin-gated read
// is reachable by admin (card stripped) but not by a member (403); cancel is
// owner-only (admin/member 403, owner past the gate).
func TestAuthorizationContract_Billing(t *testing.T) {
	db := connectTestDB(t)
	w := seedAuthzWorld(t, db)
	seedActiveSubscription(t, db, w.org.ID)
	router := buildAuthzRouter(db)

	const subPath = "/v1/billing/subscription"

	// Read: owner sees card detail; admin sees it stripped; member denied.
	oResp := authzReq(router, w, w.callerO(), http.MethodGet, subPath, nil)
	if oResp.Code != http.StatusOK || !strings.Contains(oResp.Body.String(), "4242") {
		t.Fatalf("owner subscription: code=%d wantCard; body=%s", oResp.Code, oResp.Body.String())
	}
	aResp := authzReq(router, w, w.callerA(), http.MethodGet, subPath, nil)
	if aResp.Code != http.StatusOK {
		t.Fatalf("admin subscription: got %d want 200; body=%s", aResp.Code, aResp.Body.String())
	}
	if strings.Contains(aResp.Body.String(), "4242") {
		t.Fatalf("LEAK: admin subscription exposes card last4; body=%s", aResp.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodGet, subPath, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("member subscription: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}

	// Money-move (cancel): owner-only.
	const cancelPath = "/v1/billing/subscription/cancel"
	if rr := authzReq(router, w, w.callerA(), http.MethodPost, cancelPath, map[string]any{}); rr.Code != http.StatusForbidden {
		t.Fatalf("admin cancel: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodPost, cancelPath, map[string]any{}); rr.Code != http.StatusForbidden {
		t.Fatalf("member cancel: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerO(), http.MethodPost, cancelPath, map[string]any{}); rr.Code == http.StatusForbidden {
		t.Fatalf("owner cancel: got 403, must pass the owner gate; body=%s", rr.Body.String())
	}
}

// TestAuthorizationContract_MemberLifecycle covers matrix area 8 (role changes):
// role changes are admin-gated; granting/altering the owner role is owner-only;
// a caller cannot change their own role; the sole owner is protected.
func TestAuthorizationContract_MemberLifecycle(t *testing.T) {
	db := connectTestDB(t)
	w := seedAuthzWorld(t, db)
	router := buildAuthzRouter(db)

	roleOf := func(u model.User) string {
		return "/v1/orgs/current/members/" + u.ID.String() + "/role"
	}

	// A member cannot change roles at all (route admin gate).
	if rr := authzReq(router, w, w.callerM1(), http.MethodPatch, roleOf(w.m2), map[string]any{"role": "admin"}); rr.Code != http.StatusForbidden {
		t.Fatalf("member role-change: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	// Admin may promote/demote a member between member<->admin.
	if rr := authzReq(router, w, w.callerA(), http.MethodPatch, roleOf(w.m2), map[string]any{"role": "admin"}); rr.Code != http.StatusOK {
		t.Fatalf("admin promote m2->admin: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerA(), http.MethodPatch, roleOf(w.m2), map[string]any{"role": "member"}); rr.Code != http.StatusOK {
		t.Fatalf("admin demote m2->member: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	// Only an owner may grant the owner role.
	if rr := authzReq(router, w, w.callerA(), http.MethodPatch, roleOf(w.m2), map[string]any{"role": "owner"}); rr.Code != http.StatusForbidden {
		t.Fatalf("admin grant owner: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	// An admin may not change an owner's role (owner protection).
	if rr := authzReq(router, w, w.callerA(), http.MethodPatch, roleOf(w.owner), map[string]any{"role": "admin"}); rr.Code != http.StatusForbidden {
		t.Fatalf("admin demote owner: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	// A caller cannot change their own role.
	if rr := authzReq(router, w, w.callerO(), http.MethodPatch, roleOf(w.owner), map[string]any{"role": "admin"}); rr.Code != http.StatusBadRequest {
		t.Fatalf("owner self role-change: got %d want 400; body=%s", rr.Code, rr.Body.String())
	}
	// The owner may grant the owner role to a member (mutates last, harmless).
	if rr := authzReq(router, w, w.callerO(), http.MethodPatch, roleOf(w.m2), map[string]any{"role": "owner"}); rr.Code != http.StatusOK {
		t.Fatalf("owner grant owner: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
}

// TestAuthorizationContract_OwnerOnlyDestructive covers matrix area 8
// (ownership transfer + org delete). Both are owner-only. Uses a disposable org
// so the destructive owner-allow paths do not disturb the shared fixture.
func TestAuthorizationContract_OwnerOnlyDestructive(t *testing.T) {
	db := connectTestDB(t)
	org := model.Org{ID: uuid.New(), Name: "dispose-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create disposable org: %v", err)
	}
	owner := seedOrgUser(t, db, org.ID, "owner")
	admin := seedOrgUser(t, db, org.ID, "admin")
	target := seedOrgUser(t, db, org.ID, "member")
	t.Cleanup(func() {
		db.Where("org_id = ?", org.ID).Delete(&model.OrgMembership{})
		db.Where("id = ?", org.ID).Delete(&model.Org{})
	})
	w := authzWorld{org: org, owner: owner, admin: admin}
	router := buildAuthzRouter(db)

	// transfer-ownership: admin denied (owner gate).
	xfer := "/v1/orgs/current/transfer-ownership"
	if rr := authzReq(router, w, w.callerA(), http.MethodPost, xfer, map[string]any{"new_owner_user_id": target.ID.String()}); rr.Code != http.StatusForbidden {
		t.Fatalf("admin transfer-ownership: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	// org-delete: member and admin denied (owner gate).
	del := "/v1/orgs/current"
	if rr := authzReq(router, w, w.callerA(), http.MethodDelete, del, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("admin org-delete: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	// owner transfer-ownership succeeds (owner demoted to admin, target promoted).
	if rr := authzReq(router, w, w.callerO(), http.MethodPost, xfer, map[string]any{"new_owner_user_id": target.ID.String()}); rr.Code != http.StatusOK {
		t.Fatalf("owner transfer-ownership: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	// After transfer, the target is owner and may delete the org (destructive, last).
	targetOwner := caller{user: &target}
	if rr := authzReq(router, w, targetOwner, http.MethodDelete, del, nil); rr.Code != http.StatusOK {
		t.Fatalf("new-owner org-delete: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
}

// TestAuthorizationContract_CrossOrgIDOR covers matrix area 9: a principal in a
// DIFFERENT org calling CreateReconnectSession with this org's connection id is
// org-scoped to 404 — no reconnect session is minted for a foreign connection.
func TestAuthorizationContract_CrossOrgIDOR(t *testing.T) {
	db := connectTestDB(t)
	w := seedAuthzWorld(t, db)
	router := buildAuthzRouter(db)

	// A separate org whose admin will attempt to reach w.conn (in w.org).
	otherOrg := model.Org{ID: uuid.New(), Name: "other-" + uuid.NewString()[:8], RateLimit: 1000, Active: true}
	if err := db.Create(&otherOrg).Error; err != nil {
		t.Fatalf("create other org: %v", err)
	}
	otherAdmin := seedOrgUser(t, db, otherOrg.ID, "admin")
	t.Cleanup(func() {
		db.Where("org_id = ?", otherOrg.ID).Delete(&model.OrgMembership{})
		db.Where("id = ?", otherOrg.ID).Delete(&model.Org{})
	})

	cl := caller{user: &otherAdmin}
	req := httptest.NewRequest(http.MethodPost, "/v1/connections/"+w.conn.ID.String()+"/reconnect-session", nil)
	req = cl.apply(req, otherOrg) // context org = the caller's OWN org, not w.org
	rr := httptest.NewRecorder()
	router.ServeHTTP(rr, req)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("cross-org reconnect IDOR: got %d want 404; body=%s", rr.Code, rr.Body.String())
	}
}
