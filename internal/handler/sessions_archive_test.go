package handler_test

import (
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestIntegration_SessionsArchive_AllowsSessionOwnersAndMembers(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	ownerSession := h.createSession(t, fx, fx.owner, "Archive by owner")
	h.markSessionIdle(t, ownerSession.Session.ID)
	ownerArchive := h.doJSON(t, http.MethodDelete, "/v1/sessions/"+ownerSession.Session.ID, fx, fx.owner, nil)
	if ownerArchive.Code != http.StatusOK {
		t.Fatalf("owner archive status=%d body=%s", ownerArchive.Code, ownerArchive.Body.String())
	}
	ownerOut := decodeSessionMutation(t, ownerArchive)
	if ownerOut.Session.Status != "archived" {
		t.Fatalf("owner archived status=%q, want archived", ownerOut.Session.Status)
	}

	memberSession := h.createSession(t, fx, fx.owner, "Archive by member")
	h.markSessionIdle(t, memberSession.Session.ID)
	invite := h.doJSON(t, http.MethodPut, "/v1/sessions/"+memberSession.Session.ID+"/participants/"+fx.member.ID.String(), fx, fx.owner, nil)
	if invite.Code != http.StatusOK {
		t.Fatalf("invite status=%d body=%s", invite.Code, invite.Body.String())
	}
	memberArchive := h.doJSON(t, http.MethodDelete, "/v1/sessions/"+memberSession.Session.ID, fx, fx.member, nil)
	if memberArchive.Code != http.StatusOK {
		t.Fatalf("member archive status=%d body=%s", memberArchive.Code, memberArchive.Body.String())
	}
	memberOut := decodeSessionMutation(t, memberArchive)
	if memberOut.Session.Status != "archived" {
		t.Fatalf("member archived status=%q, want archived", memberOut.Session.Status)
	}
	var stored model.Session
	if err := h.db.First(&stored, "id = ?", memberSession.Session.ID).Error; err != nil {
		t.Fatalf("load archived session: %v", err)
	}
	if stored.EndedAt == nil {
		t.Fatal("archived session ended_at is nil")
	}
}

func TestIntegration_SessionsArchive_RejectsOrgOwnerWhenNotSessionParticipant(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)
	created := h.createSession(t, fx, fx.member, "Member-owned session")

	deleted := h.doJSON(t, http.MethodDelete, "/v1/sessions/"+created.Session.ID, fx, fx.owner, nil)
	if deleted.Code != http.StatusForbidden {
		t.Fatalf("org owner delete status=%d body=%s", deleted.Code, deleted.Body.String())
	}
	patched := h.doJSON(t, http.MethodPatch, "/v1/sessions/"+created.Session.ID, fx, fx.owner, map[string]any{
		"status": "archived",
	})
	if patched.Code != http.StatusForbidden {
		t.Fatalf("org owner patch status=%d body=%s", patched.Code, patched.Body.String())
	}
	var stored model.Session
	if err := h.db.First(&stored, "id = ?", created.Session.ID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if stored.Status != "active" {
		t.Fatalf("session status=%q, want active", stored.Status)
	}
}

func TestIntegration_SessionsArchive_RejectsWhenTurnInProgress(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	created := h.createSession(t, fx, fx.owner, "Busy session")

	// Simulate an in-flight agent turn.
	if err := h.db.Model(&model.Session{}).
		Where("id = ?", created.Session.ID).
		Update("agent_turn_status", model.SessionAgentTurnActive).Error; err != nil {
		t.Fatalf("set agent turn active: %v", err)
	}

	// DELETE archive is rejected while the turn is running.
	del := h.doJSON(t, http.MethodDelete, "/v1/sessions/"+created.Session.ID, fx, fx.owner, nil)
	if del.Code != http.StatusConflict {
		t.Fatalf("delete archive during turn status=%d body=%s", del.Code, del.Body.String())
	}

	// PATCH status=archived is likewise rejected.
	patch := h.doJSON(t, http.MethodPatch, "/v1/sessions/"+created.Session.ID, fx, fx.owner, map[string]any{
		"status": "archived",
	})
	if patch.Code != http.StatusConflict {
		t.Fatalf("patch archive during turn status=%d body=%s", patch.Code, patch.Body.String())
	}

	var stored model.Session
	if err := h.db.First(&stored, "id = ?", created.Session.ID).Error; err != nil {
		t.Fatalf("load session: %v", err)
	}
	if stored.Status != "active" {
		t.Fatalf("session status=%q, want active (archive must be blocked while turn runs)", stored.Status)
	}

	// Once the turn goes idle, archiving succeeds.
	if err := h.db.Model(&model.Session{}).
		Where("id = ?", created.Session.ID).
		Update("agent_turn_status", model.SessionAgentTurnIdle).Error; err != nil {
		t.Fatalf("set agent turn idle: %v", err)
	}
	ok := h.doJSON(t, http.MethodDelete, "/v1/sessions/"+created.Session.ID, fx, fx.owner, nil)
	if ok.Code != http.StatusOK {
		t.Fatalf("delete archive when idle status=%d body=%s", ok.Code, ok.Body.String())
	}
	out := decodeSessionMutation(t, ok)
	if out.Session.Status != "archived" {
		t.Fatalf("idle archive status=%q, want archived", out.Session.Status)
	}
}

func TestIntegration_SessionsArchive_HidesArchivedSessionsFromLists(t *testing.T) {
	h := newSessionHarness(t)
	fx := h.seed(t)

	archived := h.createSession(t, fx, fx.owner, "Hidden archived session")
	h.markSessionIdle(t, archived.Session.ID)
	rr := h.doJSON(t, http.MethodDelete, "/v1/sessions/"+archived.Session.ID, fx, fx.owner, nil)
	if rr.Code != http.StatusOK {
		t.Fatalf("archive status=%d body=%s", rr.Code, rr.Body.String())
	}
	active := h.createSession(t, fx, fx.owner, "Visible active session")

	assertSessionListIDs(t, h.doJSON(t, http.MethodGet, "/v1/sessions", fx, fx.owner, nil), []string{active.Session.ID})
	assertSessionListIDs(t, h.doJSON(t, http.MethodGet, "/v1/channels/"+fx.channel.ID.String()+"/sessions", fx, fx.owner, nil), []string{active.Session.ID})
}
