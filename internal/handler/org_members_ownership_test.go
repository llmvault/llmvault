package handler_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

const transferPath = "/v1/orgs/current/transfer-ownership"
const orgPath = "/v1/orgs/current"

func TestTransferOwnership_OwnerTransfersAndIsDemoted(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	owner := h.addMember(t, org.ID, "owner")
	target := h.addMember(t, org.ID, "member")

	rr := h.do(t, http.MethodPost, transferPath, map[string]any{"new_owner_user_id": target.String()}, &org, owner)
	if rr.Code != http.StatusOK {
		t.Fatalf("transfer: got %d, body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, target); got != "owner" {
		t.Fatalf("target should be owner, got %q", got)
	}
	if got := h.roleOf(t, org.ID, owner); got != "admin" {
		t.Fatalf("previous owner should be admin, got %q", got)
	}
	// Invariant: exactly one owner remains.
	var owners int64
	h.db.Model(&model.OrgMembership{}).Where("org_id = ? AND role = ?", org.ID, "owner").Count(&owners)
	if owners != 1 {
		t.Fatalf("expected exactly 1 owner after transfer, got %d", owners)
	}
}

func TestTransferOwnership_NonOwnerForbidden(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	_ = h.addMember(t, org.ID, "owner")
	admin := h.addMember(t, org.ID, "admin")
	target := h.addMember(t, org.ID, "member")

	rr := h.do(t, http.MethodPost, transferPath, map[string]any{"new_owner_user_id": target.String()}, &org, admin)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin transferring ownership: got %d, want 403; body %s", rr.Code, rr.Body.String())
	}
	if got := h.roleOf(t, org.ID, target); got != "member" {
		t.Fatalf("target role must be unchanged, got %q", got)
	}
}

func TestTransferOwnership_NonMemberTarget(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	owner := h.addMember(t, org.ID, "owner")

	rr := h.do(t, http.MethodPost, transferPath, map[string]any{"new_owner_user_id": uuid.NewString()}, &org, owner)
	if rr.Code != http.StatusNotFound {
		t.Fatalf("transfer to non-member: got %d, want 404; body %s", rr.Code, rr.Body.String())
	}
}

func TestDeleteOrg_OwnerDeletes(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	owner := h.addMember(t, org.ID, "owner")

	rr := h.do(t, http.MethodDelete, orgPath, nil, &org, owner)
	if rr.Code != http.StatusOK {
		t.Fatalf("owner deleting org: got %d, body %s", rr.Code, rr.Body.String())
	}
	var count int64
	h.db.Model(&model.Org{}).Where("id = ?", org.ID).Count(&count)
	if count != 0 {
		t.Fatalf("org should be deleted, still present (%d)", count)
	}
	// Membership rows (no cascade FK) must also be gone.
	h.db.Model(&model.OrgMembership{}).Where("org_id = ?", org.ID).Count(&count)
	if count != 0 {
		t.Fatalf("org memberships should be deleted, %d remain", count)
	}
}

func TestDeleteOrg_NonOwnerForbidden(t *testing.T) {
	h := newOrgMemberHarness(t)
	org := createTestOrg(t, h.db)
	_ = h.addMember(t, org.ID, "owner")
	admin := h.addMember(t, org.ID, "admin")

	rr := h.do(t, http.MethodDelete, orgPath, nil, &org, admin)
	if rr.Code != http.StatusForbidden {
		t.Fatalf("admin deleting org: got %d, want 403; body %s", rr.Code, rr.Body.String())
	}
	var count int64
	h.db.Model(&model.Org{}).Where("id = ?", org.ID).Count(&count)
	if count != 1 {
		t.Fatalf("org must still exist, got count %d", count)
	}
}
