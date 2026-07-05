package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

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
		TriggerKey:   model.TriggerKeyGitHubIssueMention,
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
		{"authored by another bot", githubIssueCommentPayload("usehivy/hivy", "dependabot[bot]", "hey @hivy", false)},
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

// The split issue/PR triggers share issue_comment.created, so the entity gate
// must drop the mismatched entity before touching the DB. A payload that clears
// every other filter (right repo, human author, mentions Hivy) isolates the
// gate: if it failed to skip, deliverGitHubMention would reach loadTriggerAgent
// and panic on the zero-value handler's nil db.
func TestDeliverGitHubMentionEntityGate(t *testing.T) {
	h := &AgentTriggerDispatchHandler{}
	trigger := func(key string) model.AgentTrigger {
		return model.AgentTrigger{ID: uuid.New(), AgentID: uuid.New(), TriggerKey: key, TriggerValue: "usehivy/hivy"}
	}
	dispatch := AgentTriggerDispatchPayload{Provider: "github-app", EventType: "issue_comment", EventAction: "created"}

	cases := []struct {
		name    string
		trigger model.AgentTrigger
		payload map[string]any
	}{
		{"issue-only trigger drops PR comment", trigger(model.TriggerKeyGitHubIssueMention),
			githubIssueCommentPayload("usehivy/hivy", "alice", "hey @hivy", true)},
		{"pr-only trigger drops plain-issue comment", trigger(model.TriggerKeyGitHubPRMention),
			githubIssueCommentPayload("usehivy/hivy", "alice", "hey @hivy", false)},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := h.deliverGitHubMention(context.Background(), dispatch, tc.trigger, tc.payload); err != nil {
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

func TestIsGitHubBotAuthor(t *testing.T) {
	cases := []struct {
		login    string
		userType string
		want     bool
	}{
		{"dependabot[bot]", "Bot", true},
		{"usehivy[bot]", "", true},
		{"someuser", "Bot", true},
		{"alice", "User", false},
		{"", "", false},
	}
	for _, tc := range cases {
		if got := isGitHubBotAuthor(tc.login, tc.userType); got != tc.want {
			t.Errorf("isGitHubBotAuthor(%q, %q)=%v want %v", tc.login, tc.userType, got, tc.want)
		}
	}
}
