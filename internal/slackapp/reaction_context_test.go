package slackapp

import (
	"context"
	"testing"

	slacksdk "github.com/slack-go/slack"
)

func TestFetchReactionMessageContextFetchesThread(t *testing.T) {
	client := &reactionContextTestClient{
		history: []slacksdk.Message{{
			Msg: slacksdk.Msg{
				Timestamp:       "1599529504.000400",
				ThreadTimestamp: "1599529504.000400",
				ReplyCount:      1,
				User:            "U111",
				Text:            "please review this",
			},
		}},
		replies: []slacksdk.Message{
			{Msg: slacksdk.Msg{Timestamp: "1599529504.000400", User: "U111", Text: "please review this"}},
			{Msg: slacksdk.Msg{Timestamp: "1599529600.000100", User: "U222", Text: "looks good"}},
		},
	}

	result, err := FetchReactionMessageContext(context.Background(), client, "C111", "1599529504.000400")

	if err != nil {
		t.Fatalf("FetchReactionMessageContext error: %v", err)
	}
	if result.ThreadTS != "1599529504.000400" || len(result.ThreadMessages) != 2 {
		t.Fatalf("thread ts/messages=%q/%d", result.ThreadTS, len(result.ThreadMessages))
	}
	if client.historyLatest != "1599529504.000400" || !client.historyInclusive || client.historyLimit != 1 {
		t.Fatalf("history params latest=%q inclusive=%v limit=%d", client.historyLatest, client.historyInclusive, client.historyLimit)
	}
}

func TestFetchReactionMessageContextFetchesReactedThreadReply(t *testing.T) {
	client := &reactionContextTestClient{
		reaction: slacksdk.ReactedItem{
			Item: slacksdk.Item{
				Type:    "message",
				Channel: "C111",
				Message: &slacksdk.Message{Msg: slacksdk.Msg{
					Timestamp:       "1599529600.000100",
					ThreadTimestamp: "1599529504.000400",
					User:            "U222",
					Text:            "looks good",
				}},
			},
		},
		replies: []slacksdk.Message{
			{Msg: slacksdk.Msg{Timestamp: "1599529504.000400", User: "U111", Text: "please review this"}},
			{Msg: slacksdk.Msg{Timestamp: "1599529600.000100", ThreadTimestamp: "1599529504.000400", User: "U222", Text: "looks good"}},
		},
	}

	result, err := FetchReactionMessageContext(context.Background(), client, "C111", "1599529600.000100")

	if err != nil {
		t.Fatalf("FetchReactionMessageContext error: %v", err)
	}
	if result.Message.Timestamp != "1599529600.000100" || result.ThreadTS != "1599529504.000400" {
		t.Fatalf("message/thread ts=%q/%q", result.Message.Timestamp, result.ThreadTS)
	}
	if len(result.ThreadMessages) != 2 {
		t.Fatalf("thread messages=%d, want 2", len(result.ThreadMessages))
	}
	if client.reactionItem.Channel != "C111" || client.reactionItem.Timestamp != "1599529600.000100" || !client.reactionFull {
		t.Fatalf("reaction params=%+v full=%v", client.reactionItem, client.reactionFull)
	}
	if client.historyLatest != "" {
		t.Fatalf("history should not be used after reactions.get, latest=%q", client.historyLatest)
	}
}

func TestFetchInboundMessageContextFetchesThreadReply(t *testing.T) {
	client := &reactionContextTestClient{
		replies: []slacksdk.Message{
			{Msg: slacksdk.Msg{Timestamp: "1599529504.000400", User: "U111", Text: "please review this"}},
			{Msg: slacksdk.Msg{Timestamp: "1599529600.000100", ThreadTimestamp: "1599529504.000400", User: "U222", Text: "looks good"}},
		},
	}

	result, err := FetchInboundMessageContext(context.Background(), client, "C111", "1599529504.000400", "1599529600.000100")

	if err != nil {
		t.Fatalf("FetchInboundMessageContext error: %v", err)
	}
	if result.Message.Timestamp != "1599529600.000100" || result.ThreadTS != "1599529504.000400" {
		t.Fatalf("message/thread ts=%q/%q", result.Message.Timestamp, result.ThreadTS)
	}
	if len(result.ThreadMessages) != 2 {
		t.Fatalf("thread messages=%d, want 2", len(result.ThreadMessages))
	}
	if client.historyLatest != "" {
		t.Fatalf("history should not be used for thread replies, latest=%q", client.historyLatest)
	}
}

type reactionContextTestClient struct {
	history          []slacksdk.Message
	replies          []slacksdk.Message
	reaction         slacksdk.ReactedItem
	reactionItem     slacksdk.ItemRef
	reactionFull     bool
	historyLatest    string
	historyInclusive bool
	historyLimit     int
	repliesTimestamp string
}

func (c *reactionContextTestClient) PostMessageContext(context.Context, string, ...slacksdk.MsgOption) (string, string, error) {
	return "", "", nil
}

func (c *reactionContextTestClient) SetAssistantThreadsStatusContext(context.Context, slacksdk.AssistantThreadsSetStatusParameters) error {
	return nil
}

func (c *reactionContextTestClient) GetConversationInfoContext(context.Context, *slacksdk.GetConversationInfoInput) (*slacksdk.Channel, error) {
	return nil, nil
}

func (c *reactionContextTestClient) GetConversationHistoryContext(_ context.Context, params *slacksdk.GetConversationHistoryParameters) (*slacksdk.GetConversationHistoryResponse, error) {
	c.historyLatest = params.Latest
	c.historyInclusive = params.Inclusive
	c.historyLimit = params.Limit
	return &slacksdk.GetConversationHistoryResponse{Messages: c.history}, nil
}

func (c *reactionContextTestClient) GetConversationRepliesContext(_ context.Context, params *slacksdk.GetConversationRepliesParameters) ([]slacksdk.Message, bool, string, error) {
	c.repliesTimestamp = params.Timestamp
	return c.replies, false, "", nil
}

func (c *reactionContextTestClient) GetReactionsContext(_ context.Context, item slacksdk.ItemRef, params slacksdk.GetReactionsParameters) (slacksdk.ReactedItem, error) {
	c.reactionItem = item
	c.reactionFull = params.Full
	return c.reaction, nil
}
