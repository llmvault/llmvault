package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

// githubPROpenedPayload builds a minimal pull_request.opened webhook payload.
// The author type is derived from a "[bot]" login suffix, matching GitHub's
// user.type convention.
func githubPROpenedPayload(repo, author, body string) map[string]any {
	authorType := "User"
	if strings.HasSuffix(author, "[bot]") {
		authorType = "Bot"
	}
	return map[string]any{
		"action":     "opened",
		"number":     float64(42),
		"repository": map[string]any{"full_name": repo},
		"pull_request": map[string]any{
			"number":   float64(42),
			"title":    "Add rate limiting to the login endpoint",
			"html_url": "https://github.com/" + repo + "/pull/42",
			"user":     map[string]any{"login": author, "type": authorType},
			"body":     body,
		},
		"sender": map[string]any{"login": author, "type": authorType},
	}
}

func seedPROpenedTrigger(t *testing.T, db *gorm.DB, orgID, agentID uuid.UUID, repo string) model.AgentTrigger {
	t.Helper()
	trigger := model.AgentTrigger{
		ID: uuid.New(), OrgID: orgID, AgentID: agentID, TriggerType: "webhook",
		TriggerKey: model.TriggerKeyGitHubPROpened, TriggerValue: repo, Enabled: true,
	}
	if err := db.Create(&trigger).Error; err != nil {
		t.Fatalf("create pr-opened trigger: %v", err)
	}
	return trigger
}

func crPROpenedPayload(orgID, connID uuid.UUID, deliveryID string) AgentTriggerDispatchPayload {
	return AgentTriggerDispatchPayload{
		Provider: "github-app-code-reviews", EventType: "pull_request", EventAction: "opened",
		DeliveryID: deliveryID, OrgID: orgID, ConnectionID: connID,
	}
}

// isGitHubPROpenedTrigger is code-reviews-only and never a mention key.
func TestIsGitHubPROpenedTrigger(t *testing.T) {
	prOpened := model.AgentTrigger{TriggerKey: model.TriggerKeyGitHubPROpened}
	mention := model.AgentTrigger{TriggerKey: model.TriggerKeyGitHubPRMention}

	if !isGitHubPROpenedTrigger("github-app-code-reviews", prOpened) {
		t.Fatal("pr_opened on code-reviews app should be a pr-opened trigger")
	}
	if isGitHubPROpenedTrigger("github-app", prOpened) {
		t.Fatal("pr_opened on the primary app must NOT be a pr-opened trigger")
	}
	if isGitHubPROpenedTrigger("github-app-code-reviews", mention) {
		t.Fatal("pr_mention must not be handled by the pr-opened branch")
	}
	// pr_opened is deliberately not a mention key.
	if model.IsGitHubMentionKey(model.TriggerKeyGitHubPROpened) {
		t.Fatal("pr_opened must not be a mention key")
	}
	if isGitHubMentionTrigger("github-app-code-reviews", prOpened) {
		t.Fatal("pr_opened must not be handled by the mention branch")
	}
}

// A PR opened by a normal user, with NO @mention in the body, still delivers a
// review run in its own PR-scoped session.
func TestDeliverGitHubPROpenedDeliversWithoutMention(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
	trigger := seedPROpenedTrigger(t, db, org.ID, agent.ID, "acme/repo")
	connID := seedGitHubAppConnection(t, db, org.ID, "github-app-code-reviews", "usehivy-reviews")

	payload := crPROpenedPayload(org.ID, connID, "cr-open-1")
	webhook := githubPROpenedPayload("acme/repo", "octocat", "This adds rate limiting. No mention here.")

	if err := h.deliverGitHubPROpened(context.Background(), payload, trigger, webhook); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if got := countOrgQueue(t, db, org.ID); got != 1 {
		t.Fatalf("queued = %d, want 1", got)
	}
	// Delivered into the PR-scoped session (never a build session).
	var session model.Session
	if err := db.Where("org_id = ? AND source = ? AND source_resource_key = ?",
		org.ID, triggerConversationSource, "github/acme/repo/pull/42").First(&session).Error; err != nil {
		t.Fatalf("expected PR-scoped review session: %v", err)
	}
}

// A repository mismatch skips before any DB work / delivery.
func TestDeliverGitHubPROpenedRepoMismatchSkips(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
	trigger := seedPROpenedTrigger(t, db, org.ID, agent.ID, "acme/repo")
	connID := seedGitHubAppConnection(t, db, org.ID, "github-app-code-reviews", "usehivy-reviews")

	payload := crPROpenedPayload(org.ID, connID, "cr-open-mismatch")
	webhook := githubPROpenedPayload("other/repo", "octocat", "unrelated repo")

	if err := h.deliverGitHubPROpened(context.Background(), payload, trigger, webhook); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if got := countOrgQueue(t, db, org.ID); got != 0 {
		t.Fatalf("queued = %d, want 0 (repo mismatch must skip)", got)
	}
}

// The self-loop guard: a PR authored by the code-reviews bot itself is skipped,
// but a PR authored by the PRIMARY Hivy app (usehivy[bot], i.e. a build-agent
// PR) or another bot (dependabot) DELIVERS — reviewing those is the point.
func TestDeliverGitHubPROpenedBotAuthorPolicy(t *testing.T) {
	cases := []struct {
		name   string
		author string
		want   int64
	}{
		{"code-reviews bot self skips", "usehivy-reviews[bot]", 0},
		{"primary hivy bot delivers", "usehivy[bot]", 1},
		{"dependabot delivers", "dependabot[bot]", 1},
		{"human delivers", "alice", 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := connectTestDB(t)
			org, agent, _ := seedTriggerSessionFixture(t, db)
			h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
			trigger := seedPROpenedTrigger(t, db, org.ID, agent.ID, "acme/repo")
			connID := seedGitHubAppConnection(t, db, org.ID, "github-app-code-reviews", "usehivy-reviews")

			payload := crPROpenedPayload(org.ID, connID, "cr-open-"+tc.name)
			webhook := githubPROpenedPayload("acme/repo", tc.author, "opened by "+tc.author)

			if err := h.deliverGitHubPROpened(context.Background(), payload, trigger, webhook); err != nil {
				t.Fatalf("deliver: %v", err)
			}
			if got := countOrgQueue(t, db, org.ID); got != tc.want {
				t.Fatalf("author %q queued = %d, want %d", tc.author, got, tc.want)
			}
		})
	}
}

// The eyes ack is posted to the issue-body reactions endpoint for the PR number,
// under the delivering CODE-REVIEWS connection's identity (its Provider-Config-Key
// = the integration UniqueKey), so the ack shows as @usehivy-reviews.
func TestDeliverGitHubPROpenedEyesReactionUnderCRConnection(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	client, rc := newReactionCapture(t)
	h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}, nangoClient: client}
	trigger := seedPROpenedTrigger(t, db, org.ID, agent.ID, "acme/repo")
	connID := seedGitHubAppConnection(t, db, org.ID, "github-app-code-reviews", "usehivy-reviews")

	// The unique key the reaction must authenticate with is the CR connection's
	// integration UniqueKey.
	var conn model.Connection
	if err := db.Preload("Integration").First(&conn, "id = ?", connID).Error; err != nil {
		t.Fatalf("load connection: %v", err)
	}
	wantKey := conn.Integration.UniqueKey

	payload := crPROpenedPayload(org.ID, connID, "cr-open-eyes")
	webhook := githubPROpenedPayload("acme/repo", "octocat", "please review")

	if err := h.deliverGitHubPROpened(context.Background(), payload, trigger, webhook); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	if rc.count() != 1 {
		t.Fatalf("reaction calls = %d, want 1", rc.count())
	}
	call := rc.calls[0]
	if call.path != "/repos/acme/repo/issues/42/reactions" {
		t.Fatalf("reaction path = %q, want /repos/acme/repo/issues/42/reactions", call.path)
	}
	if call.providerConfigKey != wantKey {
		t.Fatalf("reaction Provider-Config-Key = %q, want %q (code-reviews connection)", call.providerConfigKey, wantKey)
	}
	assertEyesBody(t, call.body)
}

// Redelivery of the same pull_request.opened event dedups to a single queue row.
func TestDeliverGitHubPROpenedRedeliveryDedups(t *testing.T) {
	db := connectTestDB(t)
	org, agent, _ := seedTriggerSessionFixture(t, db)
	h := &AgentTriggerDispatchHandler{db: db, enqueuer: &fakeTaskEnqueuer{}}
	trigger := seedPROpenedTrigger(t, db, org.ID, agent.ID, "acme/repo")
	connID := seedGitHubAppConnection(t, db, org.ID, "github-app-code-reviews", "usehivy-reviews")

	payload := crPROpenedPayload(org.ID, connID, "github:pull_request.opened:279147437")
	webhook := githubPROpenedPayload("acme/repo", "octocat", "first delivery")

	for i := 0; i < 2; i++ {
		if err := h.deliverGitHubPROpened(context.Background(), payload, trigger, webhook); err != nil {
			t.Fatalf("deliver %d: %v", i, err)
		}
	}
	if got := countOrgQueue(t, db, org.ID); got != 1 {
		t.Fatalf("queued after redelivery = %d, want 1", got)
	}
}
