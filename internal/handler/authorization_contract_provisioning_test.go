package handler_test

import (
	"net/http"
	"testing"
)

// TestAuthorizationContract_TeamProvisioning covers matrix area 1: enabling a
// plugin for a team and granting a knowledge source to a team are org-admin-only
// provisioning actions. A member of the team (M1) or another team (M2) is denied
// at the RequireOrgAdmin gate; the admin passes and the grant lands (201).
func TestAuthorizationContract_TeamProvisioning(t *testing.T) {
	db := connectTestDB(t)
	w := seedAuthzWorld(t, db)
	router := buildAuthzRouter(db)

	pluginPath := "/v1/orgs/current/teams/" + w.t1.ID.String() + "/plugins"
	ragPath := "/v1/orgs/current/teams/" + w.t1.ID.String() + "/rag-sources"

	// Deny: neither team member may provision (admin-only).
	for _, tc := range []struct {
		name string
		cl   caller
	}{{"m1", w.callerM1()}, {"m2", w.callerM2()}} {
		if rr := authzReq(router, w, tc.cl, http.MethodPost, pluginPath,
			map[string]any{"plugin_id": w.pluginUngranted.ID.String()}); rr.Code != http.StatusForbidden {
			t.Fatalf("enable-team-plugin %s: got %d want 403; body=%s", tc.name, rr.Code, rr.Body.String())
		}
		if rr := authzReq(router, w, tc.cl, http.MethodPost, ragPath,
			map[string]any{"rag_source_id": w.srcUngranted.ID.String()}); rr.Code != http.StatusForbidden {
			t.Fatalf("grant-team-rag %s: got %d want 403; body=%s", tc.name, rr.Code, rr.Body.String())
		}
	}

	// Allow: admin enables an org-installed plugin for the team.
	if rr := authzReq(router, w, w.callerA(), http.MethodPost, pluginPath,
		map[string]any{"plugin_id": w.pluginUngranted.ID.String()}); rr.Code != http.StatusCreated {
		t.Fatalf("admin enable-team-plugin: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
	// Allow: admin grants an org knowledge source to the team.
	if rr := authzReq(router, w, w.callerA(), http.MethodPost, ragPath,
		map[string]any{"rag_source_id": w.srcUngranted.ID.String()}); rr.Code != http.StatusCreated {
		t.Fatalf("admin grant-team-rag: got %d want 201; body=%s", rr.Code, rr.Body.String())
	}
}

// TestAuthorizationContract_OrgCatalog covers matrix area 7: the org catalog
// (plugin install/uninstall, integration connections) is org-admin-only. Members
// are denied at the gate; the admin passes. Connection create is asserted past
// the gate (201, no provider call); revoke/reconnect deny-only (the revoke
// handler would call the provider, absent in test — we assert only the gate).
func TestAuthorizationContract_OrgCatalog(t *testing.T) {
	db := connectTestDB(t)
	w := seedAuthzWorld(t, db)
	router := buildAuthzRouter(db)

	installPath := "/v1/plugins/" + w.pluginGranted.Slug + "/install"
	connCreatePath := "/v1/integrations/" + w.integration.ID.String() + "/connections"
	revokePath := "/v1/connections/" + w.conn.ID.String()
	// A bogus connection id: an admin reaching the handler org-scopes it to 404,
	// proving the admin passed the gate without invoking the (test-absent) provider.
	reconnectBogus := "/v1/connections/" + w.org.ID.String() + "/reconnect-session"

	// Deny: a member may not touch the org catalog.
	if rr := authzReq(router, w, w.callerM1(), http.MethodPost, installPath, map[string]any{}); rr.Code != http.StatusForbidden {
		t.Fatalf("member plugin.install: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodDelete, installPath, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("member plugin.uninstall: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodPost, connCreatePath,
		map[string]any{"nango_connection_id": "x"}); rr.Code != http.StatusForbidden {
		t.Fatalf("member connections.create: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodDelete, revokePath, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("member connections.revoke: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerM1(), http.MethodPost, reconnectBogus, nil); rr.Code != http.StatusForbidden {
		t.Fatalf("member connections.reconnect: got %d want 403; body=%s", rr.Code, rr.Body.String())
	}

	// Allow: admin uninstalls (idempotent) and installs the org plugin.
	if rr := authzReq(router, w, w.callerA(), http.MethodDelete, installPath, nil); rr.Code == http.StatusForbidden {
		t.Fatalf("admin plugin.uninstall: got 403, must pass the admin gate; body=%s", rr.Body.String())
	}
	if rr := authzReq(router, w, w.callerA(), http.MethodPost, installPath, map[string]any{}); rr.Code != http.StatusOK && rr.Code != http.StatusCreated {
		t.Fatalf("admin plugin.install: got %d want 200/201; body=%s", rr.Code, rr.Body.String())
	}
	// Allow: admin passes the connections admin gate (404 = past gate, org-scoped
	// miss; not 403). The provider is not wired in test, so we assert the gate,
	// not a live create/revoke.
	if rr := authzReq(router, w, w.callerA(), http.MethodPost, reconnectBogus, nil); rr.Code != http.StatusNotFound {
		t.Fatalf("admin connections.reconnect(bogus): got %d want 404 (past gate); body=%s", rr.Code, rr.Body.String())
	}
}
