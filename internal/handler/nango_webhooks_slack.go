package handler

import (
	"encoding/json"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type slackEvent struct {
	Type     string `json:"type"`
	TeamID   string `json:"team_id"`
	Channel  string `json:"channel"`
	User     string `json:"user"`
	Text     string `json:"text"`
	TS       string `json:"ts"`
	ThreadTS string `json:"thread_ts"`
	BotID    string `json:"bot_id"`
	Subtype  string `json:"subtype"`
	Hidden   bool   `json:"hidden"`
	UserName string `json:"user_name"`
}

type slackEventCallback struct {
	Type      string     `json:"type"`
	Challenge string     `json:"challenge"`
	TeamID    string     `json:"team_id"`
	EventID   string     `json:"event_id"`
	Event     slackEvent `json:"event"`
}

func isSlackProvider(conn *model.Connection) bool {
	return conn != nil && conn.Integration.Provider == slackapp.Provider
}

func slackWebhookSentryFields(stage string, wh *nangoWebhook, conn *model.Connection, payload []byte) map[string]any {
	fields := map[string]any{
		"stage": stage,
	}
	if wh != nil {
		fields["nango_connection_id"] = wh.ConnectionID
		fields["provider_config_key"] = wh.ProviderConfigKey
		fields["webhook_type"] = wh.Type
		fields["webhook_from"] = wh.From
		fields["webhook_provider"] = wh.Provider
	}
	if conn != nil {
		fields["connection_id"] = conn.ID.String()
		fields["org_id"] = conn.OrgID.String()
		fields["integration_provider"] = conn.Integration.Provider
		fields["integration_unique_key"] = conn.Integration.UniqueKey
	}
	var callback slackEventCallback
	if len(payload) > 0 && json.Unmarshal(payload, &callback) == nil {
		fields["slack_callback_type"] = callback.Type
		fields["slack_team_id"] = firstSlackString(callback.TeamID, callback.Event.TeamID)
		fields["slack_event_id"] = callback.EventID
		fields["slack_event_type"] = callback.Event.Type
		fields["slack_event_subtype"] = callback.Event.Subtype
		fields["slack_channel_id"] = callback.Event.Channel
		fields["slack_thread_ts"] = firstSlackString(callback.Event.ThreadTS, callback.Event.TS)
		fields["slack_message_ts"] = callback.Event.TS
		fields["slack_sender_id"] = callback.Event.User
		fields["slack_bot_id"] = callback.Event.BotID
		fields["slack_hidden"] = callback.Event.Hidden
		fields["slack_is_thread_reply"] = callback.Event.ThreadTS != "" && callback.Event.ThreadTS != callback.Event.TS
	}
	return fields
}

func firstSlackString(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
