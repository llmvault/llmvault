package slackapp

import (
	"strings"
	"testing"

	slacksdk "github.com/slack-go/slack"
)

func TestRenderSlackMessageMarkdownIncludesLegacyAttachmentFields(t *testing.T) {
	message := slacksdk.Message{Msg: slacksdk.Msg{
		Text:  "GlitchTip Alert",
		BotID: "B0B8NAEH1TM",
		Attachments: []slacksdk.Attachment{{
			Title:     "*fmt.wrapError: dispatch slack service discovery",
			TitleLink: "https://glitch.example.com/issues/123",
			Color:     "danger",
			Fields: []slacksdk.AttachmentField{
				{Title: "Project", Value: "api.usehivy.com", Short: true},
				{Title: "Environment", Value: "production", Short: true},
				{Title: "Server Name", Value: "ada4767dfb57"},
			},
		}},
	}}

	got := RenderSlackMessageMarkdown(message)

	for _, want := range []string{
		"Top-level text:\nGlitchTip Alert",
		"Attachments:",
		"### [*fmt.wrapError: dispatch slack service discovery](https://glitch.example.com/issues/123)",
		"**Project:** api.usehivy.com",
		"**Environment:** production",
		"**Server Name:** ada4767dfb57",
		"Color: danger",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("rendered Slack message missing %q:\n%s", want, got)
		}
	}
}

func TestSlackMessageMediaItemsFindsImagesAndAudio(t *testing.T) {
	message := slacksdk.Message{Msg: slacksdk.Msg{
		Attachments: []slacksdk.Attachment{{
			Title:    "screenshot",
			ImageURL: "https://files.slack.com/image.png",
		}},
		Files: []slacksdk.File{{
			ID:                 "F111",
			Title:              "voice note",
			Mimetype:           "audio/mpeg",
			URLPrivateDownload: "https://files.slack.com/audio.mp3",
		}},
	}}

	items := SlackMessageMediaItems(message)

	if len(items) != 2 {
		t.Fatalf("media item count=%d, want 2: %#v", len(items), items)
	}
	if items[0].Kind != "image" || items[0].Name != "screenshot" {
		t.Fatalf("first item=%#v, want screenshot image", items[0])
	}
	if items[1].Kind != "audio" || items[1].URL != "https://files.slack.com/audio.mp3" {
		t.Fatalf("second item=%#v, want audio file", items[1])
	}
}
