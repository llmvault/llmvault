package slackgateway

import (
	"context"
	"fmt"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/logging"
)

type ThreadReplyClient interface {
	PostMessageContext(context.Context, string, ...slacksdk.MsgOption) (string, string, error)
}

func PostThreadReply(ctx context.Context, client ThreadReplyClient, channelID, threadTS, text string, fields map[string]any) (string, error) {
	_, messageTS, err := client.PostMessageContext(ctx, channelID,
		slacksdk.MsgOptionText(text, false),
		slacksdk.MsgOptionBlocks(slacksdk.NewMarkdownBlock("", text)),
		slacksdk.MsgOptionTS(threadTS),
	)
	if err != nil {
		logging.CaptureWithFields(ctx, fmt.Errorf("gateway slack: post thread reply: %w", err), fields)
		logging.FromContext(ctx).WarnContext(ctx, "gateway slack: post thread reply failed",
			"channel_id", channelID,
			"thread_ts", threadTS,
			"error", err,
		)
		return "", err
	}
	logging.FromContext(ctx).InfoContext(ctx, "gateway slack: post thread reply sent",
		"channel_id", channelID,
		"thread_ts", threadTS,
		"message_ts", messageTS,
	)
	return messageTS, nil
}
