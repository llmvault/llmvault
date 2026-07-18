package handler

import (
	"context"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

// ValidateExternalResourceRoute prepares and validates the Slack-specific
// representation of a generic team external-resource route. The database
// stores no Slack-only columns: Slack channels are simply resource type
// "slack_channel" keyed by Slack's stable channel ID.
func (h *SlackResourceRouteValidator) ValidateExternalResourceRoute(ctx context.Context, conn model.Connection, resourceType, resourceKey string) error {
	if conn.Integration.Provider != slackapp.Provider {
		// The route table intentionally accepts arbitrary providers and resource
		// types. Provider adapters opt into validation as they are added; the
		// Slack adapter validates only Slack resources.
		return nil
	}
	if strings.TrimSpace(resourceType) != "slack_channel" {
		return newExternalResourceRouteValidationError("resource_type is not supported for Slack")
	}
	channelID := strings.TrimSpace(resourceKey)
	if channelID == "" {
		return newExternalResourceRouteValidationError("resource_key is required")
	}
	token, err := h.loadSlackBotTokenForConnection(ctx, conn.OrgID, conn.ID)
	if err != nil {
		return newExternalResourceRouteValidationError("active Slack connection required")
	}
	result, err := h.joinRequestedChannels(ctx, token, joinSlackChannelsRequest{ChannelIDs: []string{channelID}})
	if err != nil {
		return fmt.Errorf("prepare Slack channel: %w", err)
	}
	if result.Failed > 0 {
		return newExternalResourceRouteValidationError("%s", slackChannelJoinFailureMessage(result))
	}
	if !result.allReady {
		return newExternalResourceRouteValidationError("Slack channel is not available")
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
