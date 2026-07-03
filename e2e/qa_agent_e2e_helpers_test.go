package e2e

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

// qaAgentInstallPlugin org-installs a plugin by slug, giving the operator a
// clear remediation message when plugin sync has not picked up the definition
// yet (404). It mirrors agentSessionsInstallPlugin but reports the 404 case
// explicitly instead of dumping a raw status mismatch.
func qaAgentInstallPlugin(t *testing.T, ctx context.Context, apiBase, token, orgID, slug string) {
	t.Helper()
	status, raw := qaAgentDo(t, ctx, http.MethodPost, apiBase+"/v1/plugins/"+slug+"/install", token, orgID, nil)
	switch status {
	case http.StatusCreated:
		return
	case http.StatusNotFound:
		t.Fatalf("plugin %q not found (404); restart the api so plugin sync picks up global/plugins/%s, then re-run. body=%s", slug, slug, raw)
	default:
		t.Fatalf("install plugin %q status=%d want 201 body=%s", slug, status, raw)
	}
}

// qaAgentWaitForFinalMarker polls the events API until a final event contains
// the marker. It refreshes *token (via agentSessionsLogin) both proactively
// every 10 minutes and reactively on any 401, so the long-running QA turn is
// not cut short by access-token expiry. The refreshed token is written back
// through the pointer for subsequent callers.
func qaAgentWaitForFinalMarker(t *testing.T, ctx context.Context, apiBase string, token *string, orgID, sessionID, marker, email, password string) agentSessionsEvent {
	t.Helper()
	deadline := time.Now().Add(28 * time.Minute)
	nextRefresh := time.Now().Add(10 * time.Minute)
	var lastTypes []string
	for time.Now().Before(deadline) {
		if time.Now().After(nextRefresh) {
			*token = agentSessionsLogin(t, ctx, apiBase, email, password, orgID).AccessToken
			nextRefresh = time.Now().Add(10 * time.Minute)
			t.Logf("proactively refreshed access token before it expires")
		}
		status, raw := qaAgentDo(t, ctx, http.MethodGet, apiBase+"/v1/sessions/"+sessionID+"/events?limit=100", *token, orgID, nil)
		if status == http.StatusUnauthorized {
			t.Logf("events API returned 401; re-logging in")
			*token = agentSessionsLogin(t, ctx, apiBase, email, password, orgID).AccessToken
			nextRefresh = time.Now().Add(10 * time.Minute)
			continue
		}
		if status != http.StatusOK {
			t.Fatalf("events poll status=%d body=%s", status, raw)
		}
		var out struct {
			Data []agentSessionsEvent `json:"data"`
		}
		if err := json.Unmarshal(raw, &out); err != nil {
			t.Fatalf("decode events poll: %v body=%s", err, raw)
		}
		lastTypes = lastTypes[:0]
		for _, event := range out.Data {
			lastTypes = append(lastTypes, event.EventType)
			if event.EventType != "final" {
				continue
			}
			payloadRaw, _ := json.Marshal(event.Payload)
			if strings.Contains(string(payloadRaw), marker) {
				return event
			}
			// A final event without the marker means the session ended some
			// other way (error, turn limit) — fail fast with its content
			// instead of polling a finished session until the deadline.
			t.Fatalf("session ended without the marker; final payload: %s", payloadRaw)
		}
		t.Logf("waiting for QA marker=%s event_types=%v", marker, lastTypes)
		select {
		case <-ctx.Done():
			t.Fatalf("context expired waiting for QA marker: %v", ctx.Err())
		case <-time.After(5 * time.Second):
		}
	}
	t.Fatalf("timed out waiting for QA final marker=%s last_event_types=%v", marker, lastTypes)
	return agentSessionsEvent{}
}

// qaAgentDo issues a JSON request and returns the raw status and body so the
// caller can branch on it (401 → re-login, 404/409 → operator guidance).
// agentSessionsJSON fatals on any status mismatch, which those flows need to
// handle instead.
func qaAgentDo(t *testing.T, ctx context.Context, method, url, token, orgID string, body any) (int, []byte) {
	t.Helper()
	var reader io.Reader
	if body != nil {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("marshal %s %s: %v", method, url, err)
		}
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, reader)
	if err != nil {
		t.Fatalf("new request %s %s: %v", method, url, err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if orgID != "" {
		req.Header.Set("X-Org-ID", orgID)
	}
	// The dev api hot-reloads on source changes, briefly dropping connections
	// (EOF / connection refused). Retry transient transport errors for up to
	// two minutes, re-issuing the request each attempt.
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if body != nil {
			payload, _ := json.Marshal(body)
			req.Body = io.NopCloser(bytes.NewReader(payload))
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			if time.Now().Before(deadline) && ctx.Err() == nil {
				t.Logf("%s %s transient transport error (api reloading?): %v — retrying", method, url, err)
				time.Sleep(5 * time.Second)
				continue
			}
			t.Fatalf("%s %s failed: %v", method, url, err)
		}
		defer resp.Body.Close()
		raw, _ := io.ReadAll(resp.Body)
		return resp.StatusCode, raw
	}
}
