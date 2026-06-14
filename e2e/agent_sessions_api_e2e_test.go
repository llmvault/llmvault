package e2e

import (
	"context"
	"net/http"
	"testing"
)

type agentSessionsAgentListItem struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	IsDefault bool   `json:"is_default"`
	Sandbox   *struct {
		ID         string `json:"id"`
		Status     string `json:"status"`
		ExternalID string `json:"external_id"`
	} `json:"sandbox"`
}

type agentSessionsChannel struct {
	ID               string `json:"id"`
	Name             string `json:"name"`
	DefaultAgentID   string `json:"default_agent_id"`
	IsDefault        bool   `json:"is_default"`
	Origin           string `json:"origin"`
	ExternalProvider string `json:"external_provider"`
}

type agentSessionsMutation struct {
	Session struct {
		ID        string  `json:"id"`
		ChannelID string  `json:"channel_id"`
		AgentID   string  `json:"agent_id"`
		SandboxID *string `json:"sandbox_id"`
	} `json:"session"`
	Event  *agentSessionsEvent `json:"event"`
	Queued bool                `json:"queued"`
}

type agentSessionsDetail struct {
	Session struct {
		ID               string `json:"id"`
		ParticipantCount int64  `json:"participant_count"`
	} `json:"session"`
	Participants []agentSessionsParticipant `json:"participants"`
}

type agentSessionsParticipant struct {
	UserID   string  `json:"user_id"`
	Role     string  `json:"role"`
	JoinedAt *string `json:"joined_at"`
}

type agentSessionsEvent struct {
	ID             string         `json:"id"`
	EventType      string         `json:"event_type"`
	SequenceNumber int64          `json:"sequence_number"`
	Payload        map[string]any `json:"payload"`
}

func agentSessionsListAgents(t *testing.T, ctx context.Context, baseURL, token, orgID string) []agentSessionsAgentListItem {
	t.Helper()
	var out struct {
		Data []agentSessionsAgentListItem `json:"data"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, baseURL+"/v1/agents?limit=100", token, orgID, nil, http.StatusOK, &out)
	return out.Data
}

func agentSessionsListChannels(t *testing.T, ctx context.Context, baseURL, token, orgID string) []agentSessionsChannel {
	t.Helper()
	var out struct {
		Data []agentSessionsChannel `json:"data"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, baseURL+"/v1/channels?limit=100", token, orgID, nil, http.StatusOK, &out)
	return out.Data
}

func agentSessionsCreateSession(t *testing.T, ctx context.Context, baseURL, token, orgID, channelID, text string) agentSessionsMutation {
	t.Helper()
	var out agentSessionsMutation
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/sessions", token, orgID, map[string]any{
		"channel_id": channelID,
		"text":       text,
	}, http.StatusCreated, &out)
	return out
}

func agentSessionsSendMessage(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID, text string) agentSessionsMutation {
	t.Helper()
	return agentSessionsSendMessageStatus(t, ctx, baseURL, token, orgID, sessionID, text, http.StatusAccepted)
}

func agentSessionsSendMessageStatus(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID, text string, status int) agentSessionsMutation {
	t.Helper()
	var out agentSessionsMutation
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/sessions/"+sessionID+"/messages", token, orgID, map[string]any{
		"text": text,
	}, status, &out)
	return out
}

func agentSessionsPutParticipant(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID, userID string) agentSessionsDetail {
	t.Helper()
	var out agentSessionsDetail
	agentSessionsJSON(t, ctx, http.MethodPut, baseURL+"/v1/sessions/"+sessionID+"/participants/"+userID, token, orgID, nil, http.StatusOK, &out)
	return out
}

func agentSessionsListEvents(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID string) []agentSessionsEvent {
	t.Helper()
	var out struct {
		Data []agentSessionsEvent `json:"data"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, baseURL+"/v1/sessions/"+sessionID+"/events?limit=100", token, orgID, nil, http.StatusOK, &out)
	return out.Data
}
