package slackapp

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	slacksdk "github.com/slack-go/slack"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
)

type ConnectionGetter interface {
	GetConnection(context.Context, string, string) (map[string]any, error)
}

type Client interface {
	PostMessageContext(context.Context, string, ...slacksdk.MsgOption) (string, string, error)
	SetAssistantThreadsStatusContext(context.Context, slacksdk.AssistantThreadsSetStatusParameters) error
	GetConversationInfoContext(context.Context, *slacksdk.GetConversationInfoInput) (*slacksdk.Channel, error)
	GetConversationHistoryContext(context.Context, *slacksdk.GetConversationHistoryParameters) (*slacksdk.GetConversationHistoryResponse, error)
	GetConversationRepliesContext(context.Context, *slacksdk.GetConversationRepliesParameters) ([]slacksdk.Message, bool, string, error)
	GetReactionsContext(context.Context, slacksdk.ItemRef, slacksdk.GetReactionsParameters) (slacksdk.ReactedItem, error)
}

var webAPIHTTPClient = &http.Client{Timeout: 30 * time.Second}

func LoadBotToken(ctx context.Context, nangoClient ConnectionGetter, conn model.Connection) (string, error) {
	if nangoClient == nil {
		return "", fmt.Errorf("nango client is required")
	}
	nangoConn, err := nangoClient.GetConnection(ctx, conn.NangoConnectionID, conn.Integration.UniqueKey)
	if err != nil {
		return "", fmt.Errorf("load Slack connection credentials: %w", err)
	}
	creds, _ := nangoConn["credentials"].(map[string]any)
	for _, key := range []string{"bot_token", "access_token"} {
		if token, _ := creds[key].(string); strings.TrimSpace(token) != "" {
			return strings.TrimSpace(token), nil
		}
	}
	return "", fmt.Errorf("Slack connection credentials do not include a bot token")
}

func NewClient(botToken string) Client {
	return slacksdk.New(botToken, slacksdk.OptionHTTPClient(webAPIHTTPClient))
}

func SetAssistantStatus(ctx context.Context, client Client, channelID, threadTS string) error {
	return client.SetAssistantThreadsStatusContext(ctx, slacksdk.AssistantThreadsSetStatusParameters{
		ChannelID:       channelID,
		ThreadTS:        threadTS,
		Status:          "is thinking...",
		LoadingMessages: []string{"is thinking...", "is working on your request..."},
	})
}

func ClearAssistantStatus(ctx context.Context, client Client, channelID, threadTS string) error {
	return client.SetAssistantThreadsStatusContext(ctx, slacksdk.AssistantThreadsSetStatusParameters{
		ChannelID: channelID,
		ThreadTS:  threadTS,
		Status:    "",
	})
}

func PostThreadReply(ctx context.Context, client Client, channelID, threadTS, text string) (string, error) {
	_, messageTS, err := client.PostMessageContext(ctx, channelID,
		slacksdk.MsgOptionText(text, false),
		slacksdk.MsgOptionBlocks(slacksdk.NewMarkdownBlock("", text)),
		slacksdk.MsgOptionTS(threadTS),
	)
	if err != nil {
		return "", err
	}
	return messageTS, nil
}

func GetChannelInfo(ctx context.Context, client Client, channelID string) (Channel, error) {
	ch, err := client.GetConversationInfoContext(ctx, &slacksdk.GetConversationInfoInput{
		ChannelID:         channelID,
		IncludeLocale:     false,
		IncludeNumMembers: true,
	})
	if err != nil {
		return Channel{}, err
	}
	if ch == nil {
		return Channel{}, nil
	}
	return channelFromSlack(*ch), nil
}

var _ ConnectionGetter = (*nango.Client)(nil)
