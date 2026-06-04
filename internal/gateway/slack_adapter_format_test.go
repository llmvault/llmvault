package gateway

import (
	"context"
	"strings"
	"testing"
)

func TestSlackAdapterFormatAgentRequestStripsMentionIDs(t *testing.T) {
	adapter := NewSlackAdapter()

	req, err := adapter.FormatAgentRequest(context.Background(), InboundEnvelope{
		SenderID:  "U789",
		ChannelID: "C456",
		ThreadID:  "123.456",
		Text:      "<@U0B584UDN15> what tools do you have?",
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.Markdown != "Slack message:\n\nwhat tools do you have?" {
		t.Fatalf("markdown = %q", req.Markdown)
	}
	if strings.Contains(req.Markdown, "U0B584UDN15") {
		t.Fatalf("markdown leaked Slack user ID: %q", req.Markdown)
	}
}
