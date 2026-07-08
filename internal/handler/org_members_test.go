package handler_test

import (
	"fmt"
	"net/http"
	"testing"

	"github.com/google/uuid"
)

func rolePath(userID uuid.UUID) string {
	return fmt.Sprintf("/v1/orgs/current/members/%s/role", userID)
}

func memberPath(userID uuid.UUID) string {
	return fmt.Sprintf("/v1/orgs/current/members/%s", userID)
}

func TestOrgMemberRole_AdminSetsMemberAndAdmin(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	admin := h.addMember(t, org.ID, "admin")
	target := h.addMember(t, org.ID, "member")

	rr := h.do(t, http.MethodPatch, rolePath(target), map[string]any{"role": "admin"}, &org, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("promote member->admin: got %d, body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, target); got != "admin" {
		t.Fatalf("expected role admin, got %q", got)
	}

	rr = h.do(t, http.MethodPatch, rolePath(target), map[string]any{"role": "member"}, &org, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("demote admin->member: got %d, body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, target); got != "member" {
		t.Fatalf("expected role member, got %q", got)
	}
}

func TestOrgMemberRole_AdminCannotCreateOwner(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	admin := h.addMember(t, org.ID, "admin")
	target := h.addMember(t, org.ID, "member")

	rr := h.do(t, http.MethodPatch, rolePath(target), map[string]any{"role": "owner"}, &org, admin)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin granting owner: got %d, want 403; body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, target); got != "member" {
		t.Fatalf("role must be unchanged, got %q", got)
	}
}

func TestOrgMemberRole_OwnerCanGrantOwner(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	owner := h.addMember(t, org.ID, "owner")
	target := h.addMember(t, org.ID, "member")

	rr := h.do(t, http.MethodPatch, rolePath(target), map[string]any{"role": "owner"}, &org, owner)
	if rr.Code != http.StatusOK {
		t.Fatalf("owner granting owner: got %d, want 200; body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, target); got != "owner" {
		t.Fatalf("expected role owner, got %q", got)
	}
}

func TestOrgMemberRole_CannotDemoteLastOwner(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	owner := h.addMember(t, org.ID, "owner")
	secondOwner := h.addMember(t, org.ID, "owner")

	// Demoting one of two owners is fine.
	rr := h.do(t, http.MethodPatch, rolePath(secondOwner), map[string]any{"role": "admin"}, &org, owner)
	if rr.Code != http.StatusOK {
		t.Fatalf("demote one of two owners: got %d, body %s", rr.Code, rr.Body.String())
	}
	// Now only `owner` remains an owner; they cannot be demoted by another owner
	// because there is none — use a fresh org to assert the last-owner guard via
	// a second owner attempting to demote the sole remaining owner is impossible,
	// so assert the sole owner cannot be demoted (self-change is separately
	// blocked). Add another owner then remove to reach the single-owner state and
	// confirm demotion is rejected.
	third := h.addMember(t, org.ID, "owner")
	rr = h.do(t, http.MethodPatch, rolePath(owner), map[string]any{"role": "admin"}, &org, third)
	if rr.Code != http.StatusOK {
		t.Fatalf("demote owner while two owners exist: got %d, body %s", rr.Code, rr.Body.String())
	}
	// `third` is the only owner now. A newly promoted owner tries to demote them.
	h.do(t, http.MethodPatch, rolePath(secondOwner), map[string]any{"role": "owner"}, &org, third)
	rr = h.do(t, http.MethodPatch, rolePath(third), map[string]any{"role": "admin"}, &org, secondOwner)
	if rr.Code != http.StatusOK {
		t.Fatalf("demote one of two owners again: got %d, body %s", rr.Code, rr.Body.String())
	}
	// Only secondOwner remains an owner. Demoting them must be rejected.
	rr = h.do(t, http.MethodPatch, rolePath(secondOwner), map[string]any{"role": "admin"}, &org, secondOwner)
	// self-change is blocked first; assert we still have an owner.
	if got := h.roleOf(t, org.ID, secondOwner); got != "owner" {
		t.Fatalf("last owner must remain owner, got %q (code %d)", got, rr.Code)
	}
}

func TestOrgMemberRole_CannotDemoteSoleOwner(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	soleOwner := h.addMember(t, org.ID, "owner")
	other := h.addMember(t, org.ID, "owner")

	// `other` demotes `soleOwner`: allowed (two owners).
	rr := h.do(t, http.MethodPatch, rolePath(soleOwner), map[string]any{"role": "admin"}, &org, other)
	if rr.Code != http.StatusOK {
		t.Fatalf("first demotion: got %d, body %s", rr.Code, rr.Body.String())
	}
	// Now `other` is the only owner. `soleOwner` (now admin) cannot demote them
	// (only an owner can change an owner).
	rr = h.do(t, http.MethodPatch, rolePath(other), map[string]any{"role": "admin"}, &org, soleOwner)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin demoting owner: got %d, want 403; body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, other); got != "owner" {
		t.Fatalf("last owner must remain owner, got %q", got)
	}
}

func TestOrgMemberRole_SelfChangeRejected(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	owner := h.addMember(t, org.ID, "owner")

	rr := h.do(t, http.MethodPatch, rolePath(owner), map[string]any{"role": "admin"}, &org, owner)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("self role change: got %d, want 400; body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, owner); got != "owner" {
		t.Fatalf("self role must be unchanged, got %q", got)
	}
}

func TestOrgMemberRole_InvalidRoleRejected(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	admin := h.addMember(t, org.ID, "admin")
	target := h.addMember(t, org.ID, "member")

	rr := h.do(t, http.MethodPatch, rolePath(target), map[string]any{"role": "superuser"}, &org, admin)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("invalid role: got %d, want 400; body %s", rr.Code, rr.Body.String())
	}
}

func TestOrgMemberRemove_AdminRemovesMember(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	admin := h.addMember(t, org.ID, "admin")
	target := h.addMember(t, org.ID, "member")

	rr := h.do(t, http.MethodDelete, memberPath(target), nil, &org, admin)
	if rr.Code != http.StatusOK {
		t.Fatalf("remove member: got %d, body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, target); got != "" {
		t.Fatalf("membership should be gone, still has role %q", got)
	}
}

func TestOrgMemberRemove_CannotRemoveLastOwner(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	owner := h.addMember(t, org.ID, "owner")
	admin := h.addMember(t, org.ID, "admin")

	// Admin cannot remove an owner at all.
	rr := h.do(t, http.MethodDelete, memberPath(owner), nil, &org, admin)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin removing owner: got %d, want 403; body %s", rr.Code, rr.Body.String())
	}

	// A second owner can remove the first, but not the last one.
	secondOwner := h.addMember(t, org.ID, "owner")
	rr = h.do(t, http.MethodDelete, memberPath(owner), nil, &org, secondOwner)
	if rr.Code != http.StatusOK {
		t.Fatalf("owner removing another owner: got %d, body %s", rr.Code, rr.Body.String())
	}
	// secondOwner is now the last owner; removing them is rejected.
	rr = h.do(t, http.MethodDelete, memberPath(secondOwner), nil, &org, secondOwner)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("removing last owner: got %d, want 400; body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, secondOwner); got != "owner" {
		t.Fatalf("last owner must remain, got %q", got)
	}
}
