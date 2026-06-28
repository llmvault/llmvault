package tasks

import (
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestSlackAgentTextIncludesSenderTagForAppMention(t *testing.T) {
	got := slackAgentText(model.SlackThreadEvent{
		EventType: "app_mention",
		Text:      "Deploy is blocked.",
		SenderID:  "U123",
	})

	for _, want := range []string{
		"Slack message:",
		"sender_id: U123",
		"sender_tag: <@U123>",
		"Message:\nDeploy is blocked.",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("slack app mention text missing %q:\n%s", want, got)
		}
	}
}

func TestSlackAgentTextLeavesReactionAutomationTextUnchanged(t *testing.T) {
	text := "Slack automation triggered by reaction :eyes:."
	got := slackAgentText(model.SlackThreadEvent{
		EventType: "reaction_added",
		Text:      text,
		SenderID:  "U123",
	})

	if got != text {
		t.Fatalf("reaction automation text changed:\n%s", got)
	}
}
