package handler_test

import (
	"net/http"
	"testing"
)

// TestAuthorizationContract_AgentCRUD covers matrix area 2 against the SHIPPED
// router. Agent Create/Update/Archive were relaxed out of the RequireOrgAdmin
// group (alongside triggers/schedules); the agent handlers now enforce
// team-primary authorization themselves. The integration truth this locks in:
//   - a member may CRUD an agent in a team they belong to (200/201);
//   - a member targeting a foreign team, or a foreign-team agent, is denied (403);
//   - an unassigned (no team_id) create by a plain member is manager-only (403);
//   - an org manager (admin/owner) or API key may act in any team, or with none.
func TestAuthorizationContract_AgentCRUD(t *testing.T) {
	db := connectTestDB(t)
	w := seedAuthzWorld(t, db)
	router := buildAuthzRouter(db)

	// --- Create -------------------------------------------------------------
	// Denied: M1 into a foreign team, M2 into T1 (not theirs), and a plain
	// member with no team (unassigned agents are manager-only).
	denyCreate := []struct {
		name string
		cl   caller
		body map[string]any
	}{
		{"m1-foreign-team", w.callerM1(), map[string]any{"name": "x", "team_id": w.t2.ID.String()}},
		{"m2-t1", w.callerM2(), map[string]any{"name": "x", "team_id": w.t1.ID.String()}},
		{"m1-no-team", w.callerM1(), map[string]any{"name": "x"}},
	}
	for _, tc := range denyCreate {
		if rr := authzReq(router, w, tc.cl, http.MethodPost, "/v1/agents", tc.body); rr.Code != http.StatusForbidden {
			t.Fatalf("agent.create %s: got %d want 403; body=%s", tc.name, rr.Code, rr.Body.String())
		}
	}

	// Allowed: M1 in their OWN team, and managers anywhere / with no team.
	allowCreate := []struct {
		name string
		cl   caller
		body map[string]any
	}{
		{"m1-own-team", w.callerM1(), map[string]any{"name": "m1a", "team_id": w.t1.ID.String()}},
		{"admin-t1", w.callerA(), map[string]any{"name": "a1", "team_id": w.t1.ID.String()}},
		{"owner-t2", w.callerO(), map[string]any{"name": "a2", "team_id": w.t2.ID.String()}},
	}
	for _, tc := range allowCreate {
		if rr := authzReq(router, w, tc.cl, http.MethodPost, "/v1/agents", tc.body); rr.Code != http.StatusCreated {
			t.Fatalf("agent.create %s: got %d want 201; body=%s", tc.name, rr.Code, rr.Body.String())
		}
	}
	// Agents always belong to a team: even a manager cannot create one without.
	if rr := authzReq(router, w, w.callerA(), http.MethodPost, "/v1/agents", map[string]any{"name": "a3"}); rr.Code != http.StatusUnprocessableEntity {
		t.Fatalf("agent.create admin-no-team: got %d want 422; body=%s", rr.Code, rr.Body.String())
	}

	// --- Update -------------------------------------------------------------
	patchT1 := "/v1/agents/" + w.agentT1.ID.String()
	patchT2 := "/v1/agents/" + w.agentT2.ID.String()
	// Denied: M2 on a T1 agent, M1 on a T2 (foreign) agent.
	if rr := authzReq(router, w, w.callerM2(), http.MethodPatch, patchT1, map[string]any{"description": "edit"}); rr.Code != http.StatusForbidden {
		t.Fatalf("m2 agent.update t1: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodPatch, patchT2, map[string]any{"description": "edit"}); rr.Code != http.StatusForbidden {
		t.Fatalf("m1 agent.update foreign-team: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	// Allowed: M1 on their OWN team agent, and managers on either team.
	if rr := authzReq(router, w, w.callerM1(), http.MethodPatch, patchT1, map[string]any{"description": "edit"}); rr.Code != http.StatusOK {
		t.Fatalf("m1 agent.update own-team: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerA(), http.MethodPatch, patchT1, map[string]any{"description": "edit2"}); rr.Code != http.StatusOK {
		t.Fatalf("admin agent.update: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerO(), http.MethodPatch, patchT2, map[string]any{"description": "edit"}); rr.Code != http.StatusOK {
		t.Fatalf("owner agent.update: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}

	// --- Archive ------------------------------------------------------------
	// Denied: M2 on a T1 agent, M1 on a T2 (foreign) agent.
	if rr := authzReq(router, w, w.callerM2(), http.MethodDelete, "/v1/agents/"+w.agentT1.ID.String(), nil); rr.Code != http.StatusForbidden {
		t.Fatalf("m2 agent.archive t1: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodDelete, "/v1/agents/"+w.agentT2.ID.String(), nil); rr.Code != http.StatusForbidden {
		t.Fatalf("m1 agent.archive foreign-team: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	// Allowed: M1 archives an unassigned-to-channel T1 agent they own; managers
	// archive the remaining team agents. Each target is archived exactly once.
	if rr := authzReq(router, w, w.callerM1(), http.MethodDelete, "/v1/agents/"+w.agentT1b.ID.String(), nil); rr.Code != http.StatusOK {
		t.Fatalf("m1 agent.archive own-team: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerA(), http.MethodDelete, "/v1/agents/"+w.agentT1.ID.String(), nil); rr.Code != http.StatusOK {
		t.Fatalf("admin agent.archive: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerO(), http.MethodDelete, "/v1/agents/"+w.agentT2.ID.String(), nil); rr.Code != http.StatusOK {
		t.Fatalf("owner agent.archive: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
}

