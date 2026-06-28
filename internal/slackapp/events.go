package slackapp

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

const (
	EventAppMention    = "app_mention"
	EventMessage       = "message"
	EventReactionAdded = "reaction_added"
)

var mentionTokenRE = regexp.MustCompile(`<@[^>]+>`)

type InboundEvent struct {
	CallbackType  string
	TeamID        string
	EventID       string
	EventType     string
	EventSubtype  string
	ChannelID     string
	ChannelType   string
	UserID        string
	UserName      string
	DisplayName   string
	Text          string
	CleanText     string
	MessageTS     string
	ThreadTS      string
	MessageAt     time.Time
	IsThreadReply bool
	Raw           map[string]any
}

type ReactionAddedEvent struct {
	CallbackType string
	TeamID       string
	EventID      string
	EventType    string
	UserID       string
	Reaction     string
	ItemType     string
	ItemChannel  string
	ItemTS       string
	ItemUser     string
	EventTS      string
	MessageAt    time.Time
	Raw          map[string]any
}

type eventCallback struct {
	Type      string     `json:"type"`
	Challenge string     `json:"challenge"`
	TeamID    string     `json:"team_id"`
	EventID   string     `json:"event_id"`
	EventTime int64      `json:"event_time"`
	Event     slackEvent `json:"event"`
}

type slackEvent struct {
	Type            string           `json:"type"`
	Team            string           `json:"team"`
	TeamID          string           `json:"team_id"`
	Channel         string           `json:"channel"`
	User            string           `json:"user"`
	UserName        string           `json:"user_name"`
	DisplayName     string           `json:"display_name"`
	Text            string           `json:"text"`
	TS              string           `json:"ts"`
	ThreadTS        string           `json:"thread_ts"`
	BotID           string           `json:"bot_id"`
	Subtype         string           `json:"subtype"`
	ChannelType     string           `json:"channel_type"`
	Hidden          bool             `json:"hidden"`
	AssistantThread *assistantThread `json:"assistant_thread,omitempty"`
	Reaction        string           `json:"reaction"`
	Item            slackEventItem   `json:"item"`
	ItemUser        string           `json:"item_user"`
	EventTS         string           `json:"event_ts"`
}

type slackEventItem struct {
	Type    string `json:"type"`
	Channel string `json:"channel"`
	TS      string `json:"ts"`
}

type assistantThread struct {
	ActionToken string `json:"action_token"`
}

func DecodeInboundEvent(payload []byte) (InboundEvent, bool, error) {
	var callback eventCallback
	if err := json.Unmarshal(payload, &callback); err != nil {
		return InboundEvent{}, false, fmt.Errorf("decode slack event: %w", err)
	}
	if callback.Type == "url_verification" || callback.Type != "event_callback" {
		return InboundEvent{}, false, nil
	}
	event := callback.Event
	if event.Type != EventAppMention && event.Type != EventMessage {
		return InboundEvent{}, false, nil
	}
	if event.BotID != "" || event.Subtype == "bot_message" || event.Hidden {
		return InboundEvent{}, false, nil
	}
	if event.Type == EventMessage {
		if event.Subtype != "" || strings.TrimSpace(event.ThreadTS) == "" {
			return InboundEvent{}, false, nil
		}
	}
	if strings.TrimSpace(event.Channel) == "" {
		return InboundEvent{}, false, fmt.Errorf("slack event missing channel")
	}
	if strings.TrimSpace(event.TS) == "" {
		return InboundEvent{}, false, fmt.Errorf("slack event missing ts")
	}
	messageAt, err := ParseTimestamp(event.TS)
	if err != nil {
		return InboundEvent{}, false, err
	}
	threadTS := strings.TrimSpace(event.ThreadTS)
	if threadTS == "" {
		threadTS = strings.TrimSpace(event.TS)
	}
	teamID := firstNonEmpty(callback.TeamID, event.TeamID, event.Team)
	raw := map[string]any{
		"team_id":         teamID,
		"event_id":        callback.EventID,
		"event_type":      event.Type,
		"event_subtype":   event.Subtype,
		"channel_type":    event.ChannelType,
		"message_ts":      event.TS,
		"thread_ts":       threadTS,
		"is_thread_reply": event.ThreadTS != "" && event.ThreadTS != event.TS,
	}
	if strings.TrimSpace(event.UserName) != "" {
		raw["user_name"] = strings.TrimSpace(event.UserName)
	}
	if strings.TrimSpace(event.DisplayName) != "" {
		raw["display_name"] = strings.TrimSpace(event.DisplayName)
	}
	if event.AssistantThread != nil && event.AssistantThread.ActionToken != "" {
		raw["action_token"] = event.AssistantThread.ActionToken
	}
	return InboundEvent{
		CallbackType:  callback.Type,
		TeamID:        teamID,
		EventID:       strings.TrimSpace(callback.EventID),
		EventType:     event.Type,
		EventSubtype:  event.Subtype,
		ChannelID:     strings.TrimSpace(event.Channel),
		ChannelType:   event.ChannelType,
		UserID:        strings.TrimSpace(event.User),
		UserName:      strings.TrimSpace(event.UserName),
		DisplayName:   strings.TrimSpace(event.DisplayName),
		Text:          strings.TrimSpace(event.Text),
		CleanText:     CleanPromptText(event.Text),
		MessageTS:     strings.TrimSpace(event.TS),
		ThreadTS:      threadTS,
		MessageAt:     messageAt,
		IsThreadReply: event.ThreadTS != "" && event.ThreadTS != event.TS,
		Raw:           raw,
	}, true, nil
}

func DecodeReactionAddedEvent(payload []byte) (ReactionAddedEvent, bool, error) {
	var raw map[string]any
	if err := json.Unmarshal(payload, &raw); err != nil {
		return ReactionAddedEvent{}, false, fmt.Errorf("decode slack reaction raw event: %w", err)
	}
	var callback eventCallback
	if err := json.Unmarshal(payload, &callback); err != nil {
		return ReactionAddedEvent{}, false, fmt.Errorf("decode slack reaction event: %w", err)
	}
	if callback.Type == "url_verification" || callback.Type != "event_callback" {
		return ReactionAddedEvent{}, false, nil
	}
	event := callback.Event
	if event.Type != EventReactionAdded {
		return ReactionAddedEvent{}, false, nil
	}
	reactionAt, err := reactionEventTime(event.EventTS, callback.EventTime, event.Item.TS)
	if err != nil {
		return ReactionAddedEvent{}, false, err
	}
	itemType := strings.TrimSpace(event.Item.Type)
	itemChannel := strings.TrimSpace(event.Item.Channel)
	itemTS := strings.TrimSpace(event.Item.TS)
	if itemType == "message" {
		if itemChannel == "" {
			return ReactionAddedEvent{}, false, fmt.Errorf("slack reaction event missing item channel")
		}
		if itemTS == "" {
			return ReactionAddedEvent{}, false, fmt.Errorf("slack reaction event missing item ts")
		}
	}
	return ReactionAddedEvent{
		CallbackType: callback.Type,
		TeamID:       firstNonEmpty(callback.TeamID, event.TeamID, event.Team),
		EventID:      strings.TrimSpace(callback.EventID),
		EventType:    event.Type,
		UserID:       strings.TrimSpace(event.User),
		Reaction:     strings.Trim(strings.TrimSpace(event.Reaction), ":"),
		ItemType:     itemType,
		ItemChannel:  itemChannel,
		ItemTS:       itemTS,
		ItemUser:     strings.TrimSpace(event.ItemUser),
		EventTS:      strings.TrimSpace(event.EventTS),
		MessageAt:    reactionAt,
		Raw:          raw,
	}, true, nil
}

func CleanPromptText(text string) string {
	text = mentionTokenRE.ReplaceAllString(text, "")
	return strings.Join(strings.Fields(text), " ")
}

func ParseTimestamp(ts string) (time.Time, error) {
	parts := strings.SplitN(strings.TrimSpace(ts), ".", 2)
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return time.Time{}, fmt.Errorf("parse slack timestamp: %w", err)
	}
	var nanos int64
	if len(parts) == 2 {
		fraction := parts[1]
		if len(fraction) > 9 {
			fraction = fraction[:9]
		}
		for len(fraction) < 9 {
			fraction += "0"
		}
		nanos, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return time.Time{}, fmt.Errorf("parse slack timestamp fraction: %w", err)
		}
	}
	return time.Unix(seconds, nanos).UTC(), nil
}

func reactionEventTime(eventTS string, eventTime int64, itemTS string) (time.Time, error) {
	if strings.TrimSpace(eventTS) != "" {
		return ParseTimestamp(eventTS)
	}
	if eventTime > 0 {
		return time.Unix(eventTime, 0).UTC(), nil
	}
	if strings.TrimSpace(itemTS) != "" {
		return ParseTimestamp(itemTS)
	}
	return time.Time{}, fmt.Errorf("slack reaction event missing timestamp")
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
