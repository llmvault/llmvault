package handler

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/slackapp"
)

func (h *SlackChannelHandler) EnsureExternalChannel(ctx context.Context, orgID uuid.UUID, req ChannelExternalProvisionRequest) error {
	if req.Provider != slackapp.Provider {
		return nil
	}
	if req.ConnectionID == uuid.Nil {
		return channelProvisionError(http.StatusBadRequest, "external_connection_id is required for Slack channels", nil)
	}
	channelID := strings.TrimSpace(req.ResourceKey)
	if channelID == "" {
		return channelProvisionError(http.StatusBadRequest, "external_resource_key is required for Slack channels", nil)
	}
	token, err := h.loadSlackBotTokenForConnection(ctx, orgID, req.ConnectionID)
	if err != nil {
		return channelProvisionError(http.StatusBadRequest, "active Slack connection required", err)
	}
	result, err := h.joinRequestedChannels(ctx, token, joinSlackChannelsRequest{ChannelIDs: []string{channelID}})
	if err != nil {
		return channelProvisionError(http.StatusInternalServerError, "failed to prepare Slack channel", err)
	}
	if result.Failed > 0 {
		return channelProvisionError(http.StatusBadRequest, slackChannelJoinFailureMessage(result), nil)
	}
	if !result.allReady {
		return channelProvisionError(http.StatusBadRequest, "Slack channel is not available", nil)
	}
	return nil
}

func slackChannelJoinFailureMessage(result joinSlackChannelsResponse) string {
	if len(result.Failures) == 0 {
		return "Slack channel is not available"
	}
	failure := result.Failures[0]
	if failure.Error == "" {
		return fmt.Sprintf("Slack channel %s is not available", failure.ChannelID)
	}
	return fmt.Sprintf("Slack channel %s is not available: %s", failure.ChannelID, failure.Error)
}

func channelProvisionError(status int, message string, err error) error {
	return &ChannelExternalProvisionError{StatusCode: status, Message: message, Err: err}
}
