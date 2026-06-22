package e2e

import (
	"context"
	"net/http"
	"net/url"
	"testing"
)

type agentSessionsAgentListItem struct {
	ID              string `json:"id"`
	Name            string `json:"name"`
	IsDefault       bool   `json:"is_default"`
	SandboxStrategy string `json:"sandbox_strategy"`
	SandboxImage    string `json:"sandbox_image"`
	Sandbox         *struct {
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
	Session agentSessionsSession `json:"session"`
	Event   *agentSessionsEvent  `json:"event"`
	Queued  bool                 `json:"queued"`
}

type agentSessionsSession struct {
	ID        string  `json:"id"`
	ChannelID string  `json:"channel_id"`
	AgentID   string  `json:"agent_id"`
	SandboxID *string `json:"sandbox_id"`
}

type agentSessionsAgentMutation struct {
	Agent agentSessionsAgentListItem `json:"agent"`
}

type agentSessionsChannelMutation struct {
	Channel agentSessionsChannel `json:"channel"`
}

type agentSessionsInterrupt struct {
	SessionID   string `json:"session_id"`
	Interrupted bool   `json:"interrupted"`
	Status      string `json:"status"`
}

type agentSessionsDetail struct {
	Session struct {
		ID               string  `json:"id"`
		ChannelID        string  `json:"channel_id"`
		AgentID          string  `json:"agent_id"`
		SandboxID        *string `json:"sandbox_id"`
		ParticipantCount int64   `json:"participant_count"`
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
	EventID        string         `json:"event_id"`
	SequenceNumber int64          `json:"sequence_number"`
	RuntimeSeq     *int64         `json:"runtime_seq,omitempty"`
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

func agentSessionsInstallCatalogAgent(t *testing.T, ctx context.Context, baseURL, token, orgID, slug string) agentSessionsAgentListItem {
	t.Helper()
	var out agentSessionsAgentMutation
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/agents/catalog/"+slug+"/install", token, orgID, nil, http.StatusCreated, &out)
	if out.Agent.ID == "" {
		t.Fatalf("catalog agent install returned empty agent: %+v", out)
	}
	return out.Agent
}

func agentSessionsListChannels(t *testing.T, ctx context.Context, baseURL, token, orgID string) []agentSessionsChannel {
	t.Helper()
	var out struct {
		Data []agentSessionsChannel `json:"data"`
	}
	agentSessionsJSON(t, ctx, http.MethodGet, baseURL+"/v1/channels?limit=100", token, orgID, nil, http.StatusOK, &out)
	return out.Data
}

func agentSessionsCreateChannel(t *testing.T, ctx context.Context, baseURL, token, orgID, name, agentID string) agentSessionsChannel {
	t.Helper()
	var out agentSessionsChannelMutation
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/channels", token, orgID, map[string]any{
		"name":             name,
		"default_agent_id": agentID,
	}, http.StatusCreated, &out)
	if out.Channel.ID == "" {
		t.Fatalf("channel create returned empty channel: %+v", out)
	}
	return out.Channel
}

func agentSessionsCreateSession(t *testing.T, ctx context.Context, baseURL, token, orgID, channelID, text string) agentSessionsMutation {
	t.Helper()
	return agentSessionsCreateSessionWithPayload(t, ctx, baseURL, token, orgID, map[string]any{
		"channel_id": channelID,
		"text":       text,
	})
}

func agentSessionsCreateSessionWithPayload(t *testing.T, ctx context.Context, baseURL, token, orgID string, payload map[string]any) agentSessionsMutation {
	t.Helper()
	var out agentSessionsMutation
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/sessions", token, orgID, payload, http.StatusCreated, &out)
	return out
}

func agentSessionsGetSession(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID string) agentSessionsDetail {
	t.Helper()
	var out agentSessionsDetail
	agentSessionsJSON(t, ctx, http.MethodGet, baseURL+"/v1/sessions/"+sessionID, token, orgID, nil, http.StatusOK, &out)
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

func agentSessionsInterruptSession(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID string) agentSessionsInterrupt {
	t.Helper()
	var out agentSessionsInterrupt
	agentSessionsJSON(t, ctx, http.MethodPost, baseURL+"/v1/sessions/"+sessionID+"/interrupt", token, orgID, nil, http.StatusOK, &out)
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

func agentSessionsListAllEvents(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID string) []agentSessionsEvent {
	t.Helper()
	var all []agentSessionsEvent
	cursor := ""
	for page := 0; page < 10; page++ {
		query := url.Values{}
		query.Set("limit", "100")
		if cursor != "" {
			query.Set("cursor", cursor)
		}
		var out struct {
			Data       []agentSessionsEvent `json:"data"`
			NextCursor *string              `json:"next_cursor"`
			HasMore    bool                 `json:"has_more"`
		}
		endpoint := baseURL + "/v1/sessions/" + sessionID + "/events?" + query.Encode()
		agentSessionsJSON(t, ctx, http.MethodGet, endpoint, token, orgID, nil, http.StatusOK, &out)
		all = append(all, out.Data...)
		if !out.HasMore {
			return all
		}
		if out.NextCursor == nil || *out.NextCursor == "" {
			t.Fatalf("events page has_more without next_cursor page=%d", page)
		}
		cursor = *out.NextCursor
	}
	t.Fatalf("session events exceeded pagination safety limit")
	return all
}
