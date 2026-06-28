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

type reactionContextTestClient struct {
	history          []slacksdk.Message
	replies          []slacksdk.Message
	historyLatest    string
	historyInclusive bool
	historyLimit     int
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

func (c *reactionContextTestClient) GetConversationRepliesContext(context.Context, *slacksdk.GetConversationRepliesParameters) ([]slacksdk.Message, bool, string, error) {
	return c.replies, false, "", nil
}
