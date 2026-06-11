package slack

import (
	"context"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// SyncDocPermissions streams the current ACL for every document in
// this source. Public channels get IsPublic=true; private channels
// get member emails from conversations.members.
func (c *SlackConnector) SyncDocPermissions(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.DocExternalAccessOrFailure, error) {
	c.ctx = ctx
	out := make(chan interfaces.DocExternalAccessOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		defer close(out)
		if c.workspaceURL == "" {
			if err := c.initWorkspace(ctx); err != nil {
				interfaces.Send(ctx, out, interfaces.NewAccessFailure(entityFailure(
					"workspace", "slack: auth.test", err,
				)))
				return
			}
		}
		channels, err := c.fetchMemberChannels(ctx)
		if err != nil {
			interfaces.Send(ctx, out, interfaces.NewAccessFailure(entityFailure(
				"channels", "slack: list channels", err,
			)))
			return
		}
		c.memberChannels = channels

		for _, ch := range channels {
			access := c.channelAccess(ch)
			err, cancelled := c.streamChannelDocAccess(ctx, ch, access, out)
			if cancelled {
				return
			}
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewAccessFailure(entityFailure(
					ch.ID, "slack: stream doc access "+ch.Name, err,
				))) {
					return
				}
			}
		}
	})
	return out, nil
}

// streamChannelDocAccess returns (err, cancelled). cancelled is true when
// the consumer abandoned the stream and the caller must stop producing.
func (c *SlackConnector) streamChannelDocAccess(
	ctx context.Context,
	channel SlackChannel,
	access *interfaces.ExternalAccess,
	out chan<- interfaces.DocExternalAccessOrFailure,
) (error, bool) {
	latest := ""
	for {
		messages, hasMore, err := c.api.getChannelHistory(ctx, channel.ID, "", latest)
		if err != nil {
			return err, false
		}
		for _, msg := range messages {
			if shouldFilter(msg, c.includeBots) != "" {
				continue
			}
			docID := docIDFromMessage(channel.ID, msg)
			if !interfaces.Send(ctx, out, interfaces.NewAccessResult(&interfaces.DocExternalAccess{
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

// SyncExternalGroups streams group definitions for Slack channels.
// Each private channel is emitted as an ExternalGroup with member
// emails so the scheduler can upsert them.
func (c *SlackConnector) SyncExternalGroups(
	ctx context.Context, _ interfaces.Source,
) (<-chan interfaces.ExternalGroupOrFailure, error) {
	c.ctx = ctx
	out := make(chan interfaces.ExternalGroupOrFailure, c.channelBuf)
	goroutine.Go(ctx, func(ctx context.Context) {
		defer close(out)
		if c.workspaceURL == "" {
			if err := c.initWorkspace(ctx); err != nil {
				interfaces.Send(ctx, out, interfaces.NewGroupFailure(entityFailure(
					"workspace", "slack: auth.test", err,
				)))
				return
			}
		}
		channels, err := c.fetchMemberChannels(ctx)
		if err != nil {
			interfaces.Send(ctx, out, interfaces.NewGroupFailure(entityFailure(
				"channels", "slack: list channels", err,
			)))
			return
		}
		c.memberChannels = channels

		for _, ch := range channels {
			if ch.IsPrivate {
				memberIDs, err := c.api.conversationMembers(ctx, ch.ID)
				if err != nil {
					if !interfaces.Send(ctx, out, interfaces.NewGroupFailure(entityFailure(
						ch.ID, "slack: members "+ch.Name, err,
					))) {
						return
					}
					continue
				}
				emails := make([]string, 0, len(memberIDs))
				for _, memberID := range memberIDs {
					u, err := c.userCache.get(ctx, c.api, memberID)
					if err != nil || u == nil {
						continue
					}
					if email := userEmail(u); email != "" {
						emails = append(emails, email)
					}
				}
				if !interfaces.Send(ctx, out, interfaces.NewGroupResult(&interfaces.ExternalGroup{
					GroupID:      "slack_channel_" + ch.ID,
					DisplayName:  "#" + ch.Name,
					MemberEmails: emails,
				})) {
					return
				}
			}
		}
	})
	return out, nil
}
