package handler

import (
	"os"
	"path/filepath"
	"testing"
)

// Fixtures are the canonical octokit payload examples (see
// internal/trigger/dispatch/testdata/github/SOURCES.md).
func githubFixture(t *testing.T, name string) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Join("..", "trigger", "dispatch", "testdata", "github", name))
	if err != nil {
		t.Fatalf("read fixture %s: %v", name, err)
	}
	return body
}

func TestStableGitHubDeliveryID(t *testing.T) {
	cases := []struct {
		fixture     string
		eventType   string
		eventAction string
		want        string
	}{
		{"issue_comment.created.issue.json", "issue_comment", "created", "github:issue_comment.created:492700400"},
		{"issue_comment.created.pr.json", "issue_comment", "created", "github:issue_comment.created:492700400"},
		{"issues.opened.json", "issues", "opened", "github:issues.opened:444500041"},
		{"pull_request.opened.json", "pull_request", "opened", "github:pull_request.opened:279147437"},
		{"pull_request_review.submitted.json", "pull_request_review", "submitted", "github:pull_request_review.submitted:237895671"},
		{"pull_request_review_comment.created.json", "pull_request_review_comment", "created", "github:pull_request_review_comment.created:284312630"},
		// The suite id recurs on re-runs, so the conclusion is part of the key.
		{"check_suite.completed.json", "check_suite", "completed", "github:check_suite.completed:118578147:success"},
		// Recurring actions must not derive a payload key: distinct events
		// would wrongly collapse into one delivery.
		{"issues.labeled.json", "issues", "labeled", ""},
		{"push.json", "push", "", ""},
	}
	for _, tc := range cases {
		body := githubFixture(t, tc.fixture)
		if got := stableGitHubDeliveryID(tc.eventType, tc.eventAction, body); got != tc.want {
			t.Errorf("stableGitHubDeliveryID(%s)=%q want %q", tc.fixture, got, tc.want)
		}
	}
	if got := stableGitHubDeliveryID("issue_comment", "created", []byte("not json")); got != "" {
		t.Errorf("invalid json id=%q want empty", got)
	}
}

// The mention pipeline's payload contract is pinned to the canonical
// fixtures: field renames upstream should fail here, not in production.
func TestInferGitHubEventFromCanonicalFixtures(t *testing.T) {
	cases := []struct {
		fixture    string
		wantType   string
		wantAction string
	}{
		{"issue_comment.created.issue.json", "issue_comment", "created"},
		{"issue_comment.created.pr.json", "issue_comment", "created"},
		{"issues.opened.json", "issues", "opened"},
		{"pull_request.opened.json", "pull_request", "opened"},
		{"pull_request_review.submitted.json", "pull_request_review", "submitted"},
		{"pull_request_review_comment.created.json", "pull_request_review_comment", "created"},
		{"check_suite.completed.json", "check_suite", "completed"},
	}
	for _, tc := range cases {
		body := githubFixture(t, tc.fixture)
		gotType, gotAction := inferGitHubEventFromPayload(body)
		if gotType != tc.wantType || gotAction != tc.wantAction {
			t.Errorf("infer(%s)=%q.%q want %q.%q", tc.fixture, gotType, gotAction, tc.wantType, tc.wantAction)
		}
	}
}
