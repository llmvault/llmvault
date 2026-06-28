package tasks

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

func TestSlackReactionAutomationTextUsesRenderedMessageAndMedia(t *testing.T) {
	orgID := uuid.New()
	messageContext := slackapp.ReactionMessageContext{
		Message: slacksdk.Message{Msg: slacksdk.Msg{
			Timestamp: "1782643393.499329",
			BotID:     "B0B8NAEH1TM",
			Text:      "GlitchTip Alert",
			Attachments: []slacksdk.Attachment{{
				Title:    "*fmt.wrapError: dispatch slack service discovery",
				ImageURL: "https://files.slack.com/screenshot.png",
				Fields: []slacksdk.AttachmentField{
					{Title: "Project", Value: "api.usehivy.com"},
					{Title: "Environment", Value: "production"},
				},
			}},
		}},
		ThreadTS: "1782643393.499329",
	}
	rendered := slackRenderedMessageContextFor(context.Background(), testSlackMediaEnricher{}, "xoxb-token", orgID, messageContext)
	text := slackReactionAutomationText(model.AgentTrigger{
		TriggerValue: "mag",
		Instructions: "Investigate production errors.",
	}, model.SlackThreadEvent{
		OrgID:          orgID,
		SenderID:       "U0B5QJZCPQR",
		SlackChannelID: "C0B8WGG0FFT",
		MessageTS:      "1782643393.499329",
		Raw: model.JSON{"event": map[string]any{
			"reaction":  "mag",
			"item_user": "B0B8NAEH1TM",
		}},
	}, messageContext, rendered)

	for _, want := range []string{
		"bot:B0B8NAEH1TM [1782643393.499329]:",
		"Top-level text:\nGlitchTip Alert",
		"Attachments:",
		"**Project:** api.usehivy.com",
		"**Environment:** production",
		`<attachment type="image"`,
		"<short_description>Screenshot shows the production error.</short_description>",
		"reacted_by: <@U0B5QJZCPQR>",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("automation text missing %q:\n%s", want, text)
		}
	}
}

func TestSlackInboundContextTextUsesRenderedMessageAndMedia(t *testing.T) {
	orgID := uuid.New()
	messageContext := slackapp.ReactionMessageContext{
		Message: slacksdk.Message{Msg: slacksdk.Msg{
			Timestamp: "1782643393.499329",
			User:      "U0B5QJZCPQR",
			Text:      "<@B123> please investigate",
			Attachments: []slacksdk.Attachment{{
				Title:    "error screenshot",
				ImageURL: "https://files.slack.com/screenshot.png",
				Fields: []slacksdk.AttachmentField{
					{Title: "Project", Value: "api.usehivy.com"},
				},
			}},
		}},
		ThreadTS: "1782643393.499329",
	}
	rendered := slackRenderedMessageContextFor(context.Background(), testSlackMediaEnricher{}, "xoxb-token", orgID, messageContext)
	text := slackInboundContextText(model.SlackThreadEvent{
		OrgID:          orgID,
		EventType:      slackapp.EventAppMention,
		SenderID:       "U0B5QJZCPQR",
		SlackChannelID: "C0B8WGG0FFT",
		MessageTS:      "1782643393.499329",
		Text:           "please investigate",
		Raw: model.JSON{
			"user_name":    "dana",
			"display_name": "Dana",
		},
	}, messageContext, rendered)

	for _, want := range []string{
		"Slack inbound message context:",
		"event_type: app_mention",
		"sender_tag: <@U0B5QJZCPQR>",
		"clean_text: please investigate",
		"Top-level text:\n<@B123> please investigate",
		"**Project:** api.usehivy.com",
		`<attachment type="image"`,
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("inbound context missing %q:\n%s", want, text)
		}
	}
}

type testSlackMediaEnricher struct{}

func (testSlackMediaEnricher) EnrichSlackMedia(context.Context, string, uuid.UUID, slacksdk.Message) string {
	return `<attachment type="image" name="screenshot.png">
<short_description>Screenshot shows the production error.</short_description>
</attachment>`
}
