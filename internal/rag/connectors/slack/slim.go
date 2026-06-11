package slack

import (
	"context"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// ListAllSlim enumerates every current document ID in the source so
// the prune loop can detect source-side deletions. For each channel,
// it walks the message history and emits one SlimDocument per
// thread/message without fetching full content.
func (c *SlackConnector) ListAllSlim(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.SlimDocOrFailure, error) {
	c.ctx = ctx
	out := make(chan interfaces.SlimDocOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		defer close(out)
		if c.workspaceURL == "" {
			if err := c.initWorkspace(ctx); err != nil {
				interfaces.Send(ctx, out, interfaces.NewSlimFailure(entityFailure(
					"workspace", "slack: auth.test", err,
				)))
				return
			}
		}
		channels, err := c.fetchMemberChannels(ctx)
		if err != nil {
			interfaces.Send(ctx, out, interfaces.NewSlimFailure(entityFailure(
				"channels", "slack: list channels", err,
			)))
			return
		}
		c.memberChannels = channels

		for _, ch := range channels {
			access := c.channelAccess(ch)
			err, cancelled := c.streamChannelSlim(ctx, ch, access, out)
			if cancelled {
				return
			}
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewSlimFailure(entityFailure(
					ch.ID, "slack: slim channel "+ch.Name, err,
				))) {
					return
				}
			}
		}
	})
	return out, nil
}

// streamChannelSlim returns (err, cancelled). cancelled is true when the
// consumer abandoned the stream (ctx done during a send) and the caller
// must stop producing.
func (c *SlackConnector) streamChannelSlim(
	ctx context.Context,
	channel SlackChannel,
	access *interfaces.ExternalAccess,
	out chan<- interfaces.SlimDocOrFailure,
) (error, bool) {
	latest := ""
	for {
		messages, hasMore, err := c.api.getChannelHistory(ctx, channel.ID, "", latest)
		if err != nil {
			return err, false
		}

		seen := make(map[string]struct{})
		for _, msg := range messages {
			if shouldFilter(msg, c.includeBots) != "" {
				continue
			}
			docID := docIDFromMessage(channel.ID, msg)
			if _, ok := seen[docID]; ok {
				continue
			}
			seen[docID] = struct{}{}
			if !interfaces.Send(ctx, out, interfaces.NewSlimResult(&interfaces.SlimDocument{
				DocID:          docID,
				ExternalAccess: access,
			})) {
				return nil, true
			}
		}

		if !hasMore || len(messages) == 0 {
			break
		}
	}
	return nil, false
}
