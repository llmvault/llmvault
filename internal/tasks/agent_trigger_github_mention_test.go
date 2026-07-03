package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func githubIssueCommentPayload(repo, commenter, body string, isPR bool) map[string]any {
	issue := map[string]any{
		"number":   float64(42),
		"title":    "Login crashes on empty email",
		"html_url": "https://github.com/" + repo + "/issues/42",
		"user":     map[string]any{"login": "alice"},
		"body":     "Steps to reproduce: submit the login form with an empty email.",
	}
	if isPR {
		issue["pull_request"] = map[string]any{"url": "https://api.github.com/repos/" + repo + "/pulls/42"}
	}
	return map[string]any{
		"action":     "created",
		"repository": map[string]any{"full_name": repo},
		"issue":      issue,
		"comment": map[string]any{
			"body":     body,
			"html_url": "https://github.com/" + repo + "/issues/42#issuecomment-1",
			"user":     map[string]any{"login": commenter},
		},
		"sender": map[string]any{"login": commenter},
	}
}

func TestParseGitHubMentionEvent(t *testing.T) {
	dispatch := AgentTriggerDispatchPayload{EventType: "issue_comment", EventAction: "created"}
	event, ok := parseGitHubMentionEvent(dispatch, githubIssueCommentPayload("usehivy/hivy", "bob", "hey @hivy take a look", false))
	if !ok {
		t.Fatal("expected issue_comment.created to parse")
	}
	if event.Repo != "usehivy/hivy" || event.Number != "42" || event.MentionedBy != "bob" ||
		!event.IsComment || event.IsPR || event.OpenedBy != "alice" {
		t.Fatalf("event=%+v", event)
	}
	if event.IssueBody == "" || event.Body != "hey @hivy take a look" {
		t.Fatalf("event bodies=%+v", event)
	}

	event, ok = parseGitHubMentionEvent(dispatch, githubIssueCommentPayload("usehivy/hivy", "bob", "@hivy", true))
	if !ok || !event.IsPR {
		t.Fatalf("expected PR comment detection, event=%+v ok=%v", event, ok)
	}

	issuesOpened := AgentTriggerDispatchPayload{EventType: "issues", EventAction: "opened"}
	event, ok = parseGitHubMentionEvent(issuesOpened, map[string]any{
		"action":     "opened",
		"repository": map[string]any{"full_name": "usehivy/hivy"},
		"issue": map[string]any{
			"number":   float64(7),
			"title":    "New bug",
			"html_url": "https://github.com/usehivy/hivy/issues/7",
			"user":     map[string]any{"login": "carol"},
			"body":     "cc @usehivy-agent",
		},
	})
	if !ok || event.Number != "7" || event.MentionedBy != "carol" || event.IsPR || event.IsComment {
		t.Fatalf("issues.opened event=%+v ok=%v", event, ok)
	}

	prOpened := AgentTriggerDispatchPayload{EventType: "pull_request", EventAction: "opened"}
	event, ok = parseGitHubMentionEvent(prOpened, map[string]any{
		"action":     "opened",
		"repository": map[string]any{"full_name": "usehivy/hivy"},
		"pull_request": map[string]any{
			"number":   float64(99),
			"title":    "Fix login",
			"html_url": "https://github.com/usehivy/hivy/pull/99",
			"user":     map[string]any{"login": "dave"},
			"body":     "@hivy please review",
		},
	})
	if !ok || !event.IsPR || event.Number != "99" || event.MentionedBy != "dave" {
		t.Fatalf("pull_request.opened event=%+v ok=%v", event, ok)
	}

	if _, ok := parseGitHubMentionEvent(dispatch, map[string]any{"action": "created"}); ok {
		t.Fatal("expected missing repository to be rejected")
	}
	edited := AgentTriggerDispatchPayload{EventType: "issue_comment", EventAction: "edited"}
	if _, ok := parseGitHubMentionEvent(edited, githubIssueCommentPayload("usehivy/hivy", "bob", "@hivy", false)); ok {
		t.Fatal("expected unsupported event key to be rejected")
	}
}

func TestGithubTextMentionsHivy(t *testing.T) {
	cases := []struct {
		text string
		want bool
	}{
		{"hey @hivy can you fix this?", true},
		{"cc @usehivy", true},
		{"ping @hivy-agent please", true},
		{"@UseHivy look", true},
		{"no mention of hivy here without an at-sign", false},
		{"email me at bob@hivy.example", true}, // handle scan is intentionally permissive
		{"talk to @octocat instead", false},
		{"", false},
	}
	for _, tc := range cases {
		if got := githubTextMentionsHivy(tc.text); got != tc.want {
			t.Errorf("githubTextMentionsHivy(%q)=%v want %v", tc.text, got, tc.want)
		}
	}
}

func TestGithubMentionResourceKey(t *testing.T) {
	issue := githubMentionEvent{Repo: "usehivy/hivy", Number: "42"}
	if key := githubMentionResourceKey(issue); key != "github/usehivy/hivy/issue/42" {
		t.Fatalf("issue key=%q", key)
	}
	pr := githubMentionEvent{Repo: "usehivy/hivy", Number: "42", IsPR: true}
	if key := githubMentionResourceKey(pr); key != "github/usehivy/hivy/pull/42" {
		t.Fatalf("pr key=%q", key)
	}
}

// deliverGitHubMention must resolve every skip case before touching the
// database: the zero-value handler panics if a skip path leaks through.
func TestDeliverGitHubMentionSkips(t *testing.T) {
	h := &AgentTriggerDispatchHandler{}
	trigger := model.AgentTrigger{
		ID:           uuid.New(),
		AgentID:      uuid.New(),
		TriggerKey:   model.TriggerKeyGitHubMention,
		TriggerValue: "usehivy/hivy",
	}
	dispatch := AgentTriggerDispatchPayload{Provider: "github-app", EventType: "issue_comment", EventAction: "created"}

	cases := []struct {
		name    string
		payload map[string]any
	}{
		{"no hivy mention", githubIssueCommentPayload("usehivy/hivy", "bob", "just chatting with @octocat", false)},
		{"different repository", githubIssueCommentPayload("acme/other", "bob", "hey @hivy", false)},
		{"authored by hivy", githubIssueCommentPayload("usehivy/hivy", "usehivy[bot]", "done! cc @hivy", false)},
		{"unsupported shape", map[string]any{"action": "created"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := h.deliverGitHubMention(context.Background(), dispatch, trigger, tc.payload); err != nil {
				t.Fatalf("expected skip, got err=%v", err)
			}
		})
	}
}

func TestCompileGitHubMentionMessage(t *testing.T) {
	h := &AgentTriggerDispatchHandler{}
	trigger := model.AgentTrigger{
		ID:           uuid.New(),
		TriggerType:  "webhook",
		Instructions: "Reply with one concise comment.",
	}
	dispatch := AgentTriggerDispatchPayload{Provider: "github-app", EventType: "issue_comment", EventAction: "created", DeliveryID: "conn:guid"}
	event, ok := parseGitHubMentionEvent(dispatch, githubIssueCommentPayload("usehivy/hivy", "bob", "hey @hivy take a look", false))
	if !ok {
		t.Fatal("parse event")
	}

	compiled := h.compileGitHubMentionMessage(context.Background(), dispatch, trigger, event)
	if compiled.ResourceKey != "github/usehivy/hivy/issue/42" {
		t.Fatalf("resource key=%q", compiled.ResourceKey)
	}
	for _, want := range []string{
		"You were mentioned on GitHub.",
		"Reply with one concise comment.",
		"usehivy/hivy",
		"#42 — Login crashes on empty email",
		"[bob] hey @hivy take a look",
		"Steps to reproduce",
	} {
		if !strings.Contains(compiled.Text, want) {
			t.Fatalf("compiled text missing %q:\n%s", want, compiled.Text)
		}
	}
	if compiled.Raw["resource_key"] != compiled.ResourceKey || compiled.Raw["event_key"] != "issue_comment.created" {
		t.Fatalf("raw=%+v", compiled.Raw)
	}
}

func TestProxyResponseArray(t *testing.T) {
	if items := proxyResponseArray(map[string]any{"_raw": `[{"body":"hi"}]`}); len(items) != 1 {
		t.Fatalf("string raw items=%v", items)
	}
	if items := proxyResponseArray(map[string]any{"_raw": []any{map[string]any{"body": "hi"}}}); len(items) != 1 {
		t.Fatalf("array raw items=%v", items)
	}
	if items := proxyResponseArray(map[string]any{"message": "Not Found"}); items != nil {
		t.Fatalf("expected nil, got %v", items)
	}
	if items := proxyResponseArray(nil); items != nil {
		t.Fatalf("expected nil for nil resp, got %v", items)
	}
}
