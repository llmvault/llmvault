package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// Every compiled GitHub message must lead with the required-reply directive so
// the agent posts back on GitHub and loads the GitHub skill if needed.
func TestGitHubReplyDirectiveInCompiledMessage(t *testing.T) {
	h := &AgentTriggerDispatchHandler{}
	trigger := model.AgentTrigger{ID: uuid.New(), TriggerType: "webhook"}
	dispatch := AgentTriggerDispatchPayload{Provider: "github-app", EventType: "issue_comment", EventAction: "created"}
	event, ok := parseGitHubMentionEvent(dispatch, githubIssueCommentPayload("acme/repo", "bob", "hey @hivy", false))
	if !ok {
		t.Fatal("parse event")
	}

	text := h.compileGitHubMentionMessage(context.Background(), dispatch, trigger, event).Text

	if !strings.HasPrefix(text, "<system_message>") {
		t.Fatalf("compiled text must start with the directive, got:\n%s", text[:min(120, len(text))])
	}
	for _, want := range []string{
		"required to respond on GitHub",
		"emoji reaction",
		"git-github skill",
		"</system_message>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("directive missing %q", want)
		}
	}
}
