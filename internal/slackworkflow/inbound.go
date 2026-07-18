package slackworkflow

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/slackapp"
)

type ClaimResult struct {
	Event     model.SlackThreadEvent
	Accepted  bool
	Duplicate bool
	Reason    string
}

func ClaimInbound(ctx context.Context, db *gorm.DB, orgID, connectionID uuid.UUID, event slackapp.InboundEvent) (ClaimResult, error) {
	if db == nil {
		return ClaimResult{}, fmt.Errorf("db is required")
	}
	accepted, reason, err := inboundAllowed(ctx, db, orgID, connectionID, event)
	if err != nil {
		return ClaimResult{}, err
	}
	status := model.SlackThreadEventStatusReceived
	if !accepted {
		status = model.SlackThreadEventStatusIgnored
	}
	row := model.SlackThreadEvent{
		OrgID:          orgID,
		ConnectionID:   connectionID,
		SlackTeamID:    event.TeamID,
		SlackChannelID: event.ChannelID,
		ThreadTS:       event.ThreadTS,
		MessageTS:      event.MessageTS,
		MessageAt:      event.MessageAt,
		EventID:        event.EventID,
		EventType:      event.EventType,
		Direction:      model.SlackThreadEventDirectionInbound,
		SenderID:       event.UserID,
		Text:           event.CleanText,
		Status:         status,
		IgnoreReason:   reason,
		Raw:            event.Raw,
		ReceivedAt:     time.Now().UTC(),
	}
	result := db.WithContext(ctx).
		Clauses(clause.OnConflict{
			Columns:     []clause.Column{{Name: "connection_id"}, {Name: "event_id"}},
			TargetWhere: clause.Where{Exprs: []clause.Expression{gorm.Expr("event_id <> ?", "")}},
			DoNothing:   true,
		}).
		Create(&row)
	if result.Error != nil {
		return ClaimResult{}, fmt.Errorf("claim slack inbound: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		var existing model.SlackThreadEvent
		err := db.WithContext(ctx).
			Where("connection_id = ? AND event_id = ?", connectionID, event.EventID).
			First(&existing).Error
		if err != nil {
			return ClaimResult{}, fmt.Errorf("load claimed slack inbound: %w", err)
		}
		return ClaimResult{
			Event:     existing,
			Accepted:  existing.Status != model.SlackThreadEventStatusIgnored,
			Duplicate: true,
			Reason:    existing.IgnoreReason,
		}, nil
	}
	return ClaimResult{Event: row, Accepted: accepted, Reason: reason}, nil
}

// slackWorkspaceChannelTypes is the allowlist of Slack channel types Hivy will
// act on. Only in-workspace public ("channel") and private ("group") channels
// qualify. DM surfaces ("im", "mpim") — which are how Slack Connect exposes
// external users — are excluded so an external / Slack-Connect sender cannot
// spin up an org-billed run.
//
// ASSUMPTION (documented): there is no Slack-user → Hivy-member identity map, so
// authorization is approximated by (a) requiring a real workspace user id and
// (b) restricting to workspace channel types. This does NOT yet exclude an
// external member invited into a normally-shared channel (channel_type stays
// "channel"); fully doing so needs the installing-workspace team id vs the
// sender team id, which slackapp.DecodeInboundEvent does not currently surface
// separately (see cross-agent TODO).
var slackWorkspaceChannelTypes = map[string]bool{
	"channel": true,
	"group":   true,
}

// slackChannelSurfaceAllowed falls back to the conversation-ID prefix when the
// event carries no channel_type: app_mention payloads never include that field
// (only message events do). Slack IDs are C (public), G (private/group), or
// D (DM) — the DM surface stays excluded either way.
func slackChannelSurfaceAllowed(event slackapp.InboundEvent) bool {
	if channelType := strings.TrimSpace(event.ChannelType); channelType != "" {
		return slackWorkspaceChannelTypes[channelType]
	}
	id := strings.TrimSpace(event.ChannelID)
	return strings.HasPrefix(id, "C") || strings.HasPrefix(id, "G")
}

func inboundAllowed(ctx context.Context, db *gorm.DB, orgID, connectionID uuid.UUID, event slackapp.InboundEvent) (bool, string, error) {
	if event.EventType == slackapp.EventAppMention {
		// Authorized-sender gate: require a real workspace user and an
		// in-workspace channel type before accepting an app_mention. Previously
		// every app_mention was accepted, letting any external Slack user spend
		// org credits.
		if strings.TrimSpace(event.UserID) == "" {
			return false, "no_sender", nil
		}
		if !slackChannelSurfaceAllowed(event) {
			return false, "unsupported_channel_type", nil
		}
		return true, "", nil
	}
	if strings.TrimSpace(event.CleanText) == "" {
		return false, "empty_text", nil
	}
	if event.EventType != slackapp.EventMessage {
		return false, "unsupported_event", nil
	}
	outbound, ok, err := latestThreadEvent(ctx, db, orgID, connectionID, event, model.SlackThreadEventDirectionOutbound)
	if err != nil {
		return false, "", err
	}
	if !ok {
		return false, "no_prior_hivy_reply", nil
	}
	inbound, ok, err := latestThreadEvent(ctx, db, orgID, connectionID, event, model.SlackThreadEventDirectionInbound)
	if err != nil {
		return false, "", err
	}
	if ok && !outbound.MessageAt.After(inbound.MessageAt) {
		return false, "prior_human_message_after_hivy_reply", nil
	}
	if !event.MessageAt.After(outbound.MessageAt) {
		return false, "message_not_after_hivy_reply", nil
	}
	return true, "", nil
}

func latestThreadEvent(ctx context.Context, db *gorm.DB, orgID, connectionID uuid.UUID, event slackapp.InboundEvent, direction string) (model.SlackThreadEvent, bool, error) {
	var row model.SlackThreadEvent
	q := db.WithContext(ctx).
		Where("org_id = ? AND connection_id = ?", orgID, connectionID).
		Where("slack_channel_id = ? AND thread_ts = ? AND direction = ?", event.ChannelID, event.ThreadTS, direction).
		Order("message_at DESC")
	if direction == model.SlackThreadEventDirectionInbound {
		q = q.Where("status <> ?", model.SlackThreadEventStatusIgnored)
	} else {
		q = q.Where("status IN ?", []string{model.SlackThreadEventStatusSent, model.SlackThreadEventStatusCompleted})
	}
	err := q.First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.SlackThreadEvent{}, false, nil
	}
	if err != nil {
		return model.SlackThreadEvent{}, false, fmt.Errorf("load latest slack thread event: %w", err)
	}
	return row, true, nil
}
