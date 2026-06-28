package handler

import (
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type triggerListResponse struct {
	Data []triggerAutomationResponse `json:"data"`
}

type triggerGetResponse struct {
	Trigger triggerAutomationResponse `json:"trigger"`
}

type triggerAutomationResponse struct {
	ID                   string `json:"id"`
	TriggerType          string `json:"trigger_type"`
	Provider             string `json:"provider"`
	ConnectionID         string `json:"connection_id,omitempty"`
	ConnectionName       string `json:"connection_name,omitempty"`
	ChannelID            string `json:"channel_id,omitempty"`
	ChannelName          string `json:"channel_name,omitempty"`
	ExternalResourceKey  string `json:"external_resource_key,omitempty"`
	ExternalResourceName string `json:"external_resource_name,omitempty"`
	AgentID              string `json:"agent_id"`
	AgentName            string `json:"agent_name"`
	AgentIcon            string `json:"agent_icon,omitempty"`
	AgentAvatarURL       string `json:"agent_avatar_url,omitempty"`
	TriggerKey           string `json:"trigger_key"`
	TriggerValue         string `json:"trigger_value"`
	Enabled              bool   `json:"enabled"`
	SourceSlug           string `json:"source_slug,omitempty"`
	Instructions         string `json:"instructions,omitempty"`
	CreatedAt            string `json:"created_at"`
	UpdatedAt            string `json:"updated_at"`
}

func triggerAutomationToResponse(trigger model.AgentTrigger) triggerAutomationResponse {
	provider := triggerProvider(trigger)
	response := triggerAutomationResponse{
		ID:           trigger.ID.String(),
		TriggerType:  trigger.TriggerType,
		Provider:     provider,
		AgentID:      trigger.AgentID.String(),
		AgentName:    fallbackAgentName(trigger.Agent),
		AgentIcon:    trigger.Agent.Icon,
		TriggerKey:   trigger.TriggerKey,
		TriggerValue: trigger.TriggerValue,
		Enabled:      trigger.Enabled,
		SourceSlug:   trigger.SourceSlug,
		Instructions: trigger.Instructions,
		CreatedAt:    formatTriggerTime(trigger.CreatedAt),
		UpdatedAt:    formatTriggerTime(trigger.UpdatedAt),
	}
	if trigger.Agent.AvatarURL != nil {
		response.AgentAvatarURL = *trigger.Agent.AvatarURL
	}
	if trigger.ConnectionID != nil {
		response.ConnectionID = trigger.ConnectionID.String()
		response.ConnectionName = trigger.Connection.NangoConnectionID
	}
	if trigger.ChannelID != nil {
		response.ChannelID = trigger.ChannelID.String()
	}
	if trigger.Channel != nil {
		response.ChannelName = trigger.Channel.Name
		response.ExternalResourceKey = trigger.Channel.ExternalResourceKey
		response.ExternalResourceName = trigger.Channel.ExternalResourceName
	}
	return response
}

func triggerProvider(trigger model.AgentTrigger) string {
	if trigger.ConnectionID != nil && trigger.Connection.Integration.Provider != "" {
		return trigger.Connection.Integration.Provider
	}
	if trigger.Channel != nil && trigger.Channel.ExternalProvider != "" {
		return trigger.Channel.ExternalProvider
	}
	return ""
}

func formatTriggerTime(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format(time.RFC3339)
}

func triggerIDFromString(raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	return id, err == nil && id != uuid.Nil
}
