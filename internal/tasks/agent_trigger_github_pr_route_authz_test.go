package tasks

import (
	"context"
	"net/http"
	"testing"
)

// --- Fix 3: write-access author filter ---

func TestPRRouteWriteAccessFilter(t *testing.T) {
	cases := []struct {
		name       string
		author     string
		perm       *stubResponse
		wantRouted bool
	}{
		{"write access routes", "human", &stubResponse{body: `{"permission":"write","role_name":"write"}`}, true},
		{"admin routes", "human", &stubResponse{body: `{"permission":"admin","role_name":"admin"}`}, true},
		{"read access blocked", "driveby", &stubResponse{body: `{"permission":"read","role_name":"read"}`}, false},
		{"bot 404 blocked", "greptile-apps[bot]", &stubResponse{status: http.StatusNotFound}, false},
		{"rate-limit non-2xx blocked", "human", &stubResponse{status: http.StatusForbidden}, false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := connectTestDB(t)
			org, agent, _ := seedTriggerSessionFixture(t, db)
			orig := seedActiveSession(t, db, org.ID, agent.ID)
			seedPRMapping(t, db, org.ID, "acme/repo", 42, orig.ID)
			connID := seedGitHubConnection(t, db, org.ID)
			client, stub := newGitHubAPIStub(t)
			if tc.perm != nil {
				stub.permission[tc.author] = *tc.perm
			}
			h := newPRAPIRouteHandler(db, client)

			payload := AgentTriggerDispatchPayload{
				Provider: "github-app", EventType: "issue_comment", EventAction: "created",
				OrgID: org.ID, ConnectionID: connID,
				DeliveryID: prRouteDeliveryID("conn-a", "issue_comment.created", "492700400"),
			}
			webhook := githubIssueCommentPayload("acme/repo", tc.author, "please rebase", true)
			routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
			if err != nil {
				t.Fatalf("route: %v", err)
			}
			got := len(routed) > 0
			if got != tc.wantRouted {
				t.Fatalf("routed=%v want %v (queued=%d)", got, tc.wantRouted, countSessionQueue(t, db, orig.ID))
			}
		})
	}
}

// Hivy's own code-reviews bot is trusted and bypasses the collaborator check
// (bots are never collaborators), so its feedback still reaches the build agent.
func TestPRRouteWriteAccessExemptsHivyReviewBot(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	orig := seedActiveSession(t, db, org.ID, agent.ID)
	seedPRMapping(t, db, org.ID, "acme/repo", 9, orig.ID)
	connID := seedGitHubConnection(t, db, org.ID)
	client, stub := newGitHubAPIStub(t)
	stub.reviewCmts = stubResponse{body: `[]`}
	h := newPRAPIRouteHandler(db, client)

	payload := AgentTriggerDispatchPayload{
		Provider: "github-app", EventType: "pull_request_review", EventAction: "submitted",
		OrgID: org.ID, ConnectionID: connID,
		DeliveryID: prRouteDeliveryID("conn-a", "pull_request_review.submitted", "777000222"),
	}
	webhook := githubReviewPayload("acme/repo", 9, "usehivy-reviews[bot]", "changes_requested", "please fix the nil check", "usehivy[bot]")
	routed, err := h.maybeRoutePREvent(context.Background(), payload, webhook)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	if _, ok := routed[orig.ID]; !ok {
		t.Fatalf("code-reviews bot review not routed: %v", routed)
	}
	if len(stub.permCalls) != 0 {
		t.Fatalf("permission endpoint hit for Hivy bot: %v", stub.permCalls)
	}
}
