package handler_test

import (
	"net/http"
	"strings"
	"testing"
)

// TestAuthorizationContract_TriggersSchedules covers matrix area 5: creating a
// trigger or schedule bound to a channel is a manage-the-channel's-team action.
// A member of the channel's team (M1, T1) may bind a T1 agent to a T1 channel
// (201); a member of another team (M2) is denied (403); an org manager passes.
func TestAuthorizationContract_TriggersSchedules(t *testing.T) {
	db := connectTestDB(t)
	w := seedAuthzWorld(t, db)
	router := buildAuthzRouter(db)

	trigBody := func() map[string]any {
		return map[string]any{
			"trigger_type": "http",
			"name":         "trg-" + strings.ToLower(w.t1.ID.String()[:6]),
			"agent_id":     w.agentT1.ID.String(),
			"channel_id":   w.chT1.ID.String(),
			"instructions": "do the thing",
		}
	}
	interval := int64(3600)
	schedBody := func() map[string]any {
		return map[string]any{
			"name":             "sch",
			"agent_id":         w.agentT1.ID.String(),
			"channel_id":       w.chT1.ID.String(),
			"task_prompt":      "do the thing",
			"interval_seconds": interval,
		}
	}

	// M2 is not on chT1's team -> denied binding to it.
	if rr := authzReq(router, w, w.callerM2(), http.MethodPost, "/v1/triggers", trigBody()); rr.Code != http.StatusForbidden {
		t.Fatalf("m2 trigger on chT1: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM2(), http.MethodPost, "/v1/schedules", schedBody()); rr.Code != http.StatusForbidden {
		t.Fatalf("m2 schedule on chT1: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	// M1 (T1 member) may bind a T1 agent to a T1 channel.
	if rr := authzReq(router, w, w.callerM1(), http.MethodPost, "/v1/triggers", trigBody()); rr.Code != http.StatusCreated {
		t.Fatalf("m1 trigger on chT1: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodPost, "/v1/schedules", schedBody()); rr.Code != http.StatusCreated {
		t.Fatalf("m1 schedule on chT1: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	// An org manager (admin) passes the manage gate too.
	if rr := authzReq(router, w, w.callerA(), http.MethodPost, "/v1/triggers", trigBody()); rr.Code != http.StatusCreated {
		t.Fatalf("admin trigger on chT1: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
}

// TestAuthorizationContract_PrivateChannel covers matrix area 10: a private
// channel is reachable only by its explicit members and org managers. M2 (same
// org, not a channel member, not on the channel's team) can neither list, get,
// nor use it; M1 (an explicit channel member) and the admin can.
func TestAuthorizationContract_PrivateChannel(t *testing.T) {
	db := connectTestDB(t)
	w := seedAuthzWorld(t, db)
	router := buildAuthzRouter(db)

	privID := w.chT1Priv.ID.String()
	getPath := "/v1/channels/" + privID
	agentsPath := getPath + "/agents"

	// List: the private channel must not appear for M2, must appear for M1/admin.
	m2list := authzReq(router, w, w.callerM2(), http.MethodGet, "/v1/channels?limit=200", nil)
	if strings.Contains(m2list.Body.String(), privID) {
		t.Fatalf("LEAK: m2 channels.list contains private channel %s\nbody=%s", privID, m2list.Body.String())
	}
	m1list := authzReq(router, w, w.callerM1(), http.MethodGet, "/v1/channels?limit=200", nil)
	if !strings.Contains(m1list.Body.String(), privID) {
		t.Fatalf("m1 (channel member) channels.list missing private channel %s\nbody=%s", privID, m1list.Body.String())
	}

	// Get + use (list agents): denied for M2, allowed for M1 and admin.
	if rr := authzReq(router, w, w.callerM2(), http.MethodGet, getPath, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("m2 private channel.get: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM2(), http.MethodGet, agentsPath, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("m2 private channel.agents: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodGet, getPath, nil); rr.Code != http.StatusOK {
		t.Fatalf("m1 private channel.get: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerA(), http.MethodGet, getPath, nil); rr.Code != http.StatusOK {
		t.Fatalf("admin private channel.get: got %d want 200; body=%s", rr.Code, rr.Body.String())
	}
}

// TestAuthorizationContract_SandboxExec covers matrix area 11: exec is reachable
// only when the sandbox's agent is visible to the caller. M1 reaches a T1-agent
// sandbox (past the visibility gate; 503 here since no orchestrator is wired)
// but a T2-agent sandbox is indistinguishable from nonexistent (404).
func TestAuthorizationContract_SandboxExec(t *testing.T) {
	db := connectTestDB(t)
	w := seedAuthzWorld(t, db)
	router := buildAuthzRouter(db)

	exec := func(cl caller, id string) int {
		return authzReq(router, w, cl, http.MethodPost, "/v1/sandboxes/"+id+"/exec",
			map[string]any{"commands": []string{"echo hi"}}).Code
	}

	if got := exec(w.callerM1(), w.sbT2.ID.String()); got != http.StatusNotFound {
		t.Fatalf("m1 exec T2 sandbox: got %d want 404 (hidden agent)", got)
	}
	if got := exec(w.callerM1(), w.sbT1.ID.String()); got == http.StatusNotFound || got == http.StatusForbidden {
		t.Fatalf("m1 exec T1 sandbox: got %d, expected to pass the visibility gate (503 without orchestrator)", got)
	}
}
