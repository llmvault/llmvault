package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
)

type connectionInboundDecision struct {
	allowed bool
	reason  string
}

func (s *Service) allowConnectionInbound(ctx context.Context, envelope WebhookEnvelope, inbound InboundEnvelope) (connectionInboundDecision, error) {
	if !strings.EqualFold(envelope.Provider, SlackProvider) || !slackInboundMessageEvent(inbound) {
		return connectionInboundDecision{allowed: true}, nil
	}
	if !slackInboundThreadReply(inbound) {
		return connectionInboundDecision{reason: "slack_message_not_thread_reply"}, nil
	}

	session, found, err := s.loadActiveConnectionSession(ctx, envelope, inbound.ThreadKey)
	if err != nil {
		return connectionInboundDecision{}, err
	}
	if !found {
		return connectionInboundDecision{reason: "slack_thread_not_active"}, nil
	}

	latestHivyTS, err := s.latestSlackDeliveryTS(ctx, session)
	if err != nil {
		return connectionInboundDecision{}, err
	}
	if latestHivyTS == "" {
		return connectionInboundDecision{reason: "slack_thread_no_hivy_delivery"}, nil
	}

	latestHumanTS, err := s.latestSlackInboundTS(ctx, session)
	if err != nil {
		return connectionInboundDecision{}, err
	}
	if latestHumanTS != "" && !slackTSAfter(latestHivyTS, latestHumanTS) {
		return connectionInboundDecision{reason: "slack_hivy_not_latest"}, nil
	}
	if !slackTSAfter(inbound.ExternalMessageID, latestHivyTS) {
		return connectionInboundDecision{reason: "slack_message_before_hivy_reply"}, nil
	}
	return connectionInboundDecision{allowed: true}, nil
}

func slackInboundMessageEvent(inbound InboundEnvelope) bool {
	eventType, _ := inbound.Raw["event_type"].(string)
	return strings.EqualFold(eventType, "message")
}

func slackInboundThreadReply(inbound InboundEnvelope) bool {
	isThreadReply, _ := inbound.Raw["is_thread_reply"].(bool)
	return isThreadReply
}

func (s *Service) loadActiveConnectionSession(ctx context.Context, envelope WebhookEnvelope, threadKey string) (model.AgentSession, bool, error) {
	var session model.AgentSession
	err := s.db.WithContext(ctx).
		Where("org_id = ? AND agent_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?",
			envelope.OrgID, envelope.AgentID, Source, envelope.ConnectionID, threadKey, "active").
		First(&session).Error
	if err == nil {
		return session, true, nil
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return model.AgentSession{}, false, nil
	}
	return model.AgentSession{}, false, fmt.Errorf("load active slack thread session: %w", err)
}

func (s *Service) latestSlackInboundTS(ctx context.Context, session model.AgentSession) (string, error) {
	var events []model.AgentGatewayEvent
	err := s.db.WithContext(ctx).
		Where("agent_session_id = ? AND provider = ? AND sender_id <> ? AND status = ?",
			session.ID, SlackProvider, "", "delivered").
		Order("received_at DESC, created_at DESC").
		Limit(50).
		Find(&events).Error
	if err != nil {
		return "", fmt.Errorf("load latest slack inbound event: %w", err)
	}
	latest := ""
	for _, event := range events {
		latest = maxSlackTS(latest, event.ExternalMessageID)
	}
	return latest, nil
}

func (s *Service) latestSlackDeliveryTS(ctx context.Context, session model.AgentSession) (string, error) {
	var deliveries []model.AgentGatewayDelivery
	if err := s.db.WithContext(ctx).
		Where("agent_session_id = ? AND provider = ? AND status = ?", session.ID, SlackProvider, "sent").
		Order("created_at DESC").
		Limit(20).
		Find(&deliveries).Error; err != nil {
		return "", fmt.Errorf("load latest slack delivery: %w", err)
	}
	latest := ""
	for _, delivery := range deliveries {
		latest = maxSlackTS(latest, slackDeliveryTS(delivery))
	}
	return latest, nil
}

func slackDeliveryTS(delivery model.AgentGatewayDelivery) string {
	var handles []MessageHandle
	if err := json.Unmarshal([]byte(delivery.ProviderHandles), &handles); err != nil {
		return ""
	}
	for _, handle := range handles {
		if ts := strings.TrimSpace(handle.ProviderMessageID); ts != "" {
			return ts
		}
		if handle.Raw != nil {
			if ts, _ := handle.Raw["slack_ts"].(string); strings.TrimSpace(ts) != "" {
				return strings.TrimSpace(ts)
			}
		}
	}
	return ""
}

func slackTSAfter(candidate, previous string) bool {
	candidateSeconds, candidateMicros, ok := parseSlackTS(candidate)
	if !ok {
		return false
	}
	previousSeconds, previousMicros, ok := parseSlackTS(previous)
	if !ok {
		return false
	}
	if candidateSeconds != previousSeconds {
		return candidateSeconds > previousSeconds
	}
	return candidateMicros > previousMicros
}

func maxSlackTS(left, right string) string {
	left = strings.TrimSpace(left)
	right = strings.TrimSpace(right)
	if left == "" {
		return right
	}
	if right == "" {
		return left
	}
	if slackTSAfter(right, left) {
		return right
	}
	return left
}

func parseSlackTS(value string) (int64, int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, 0, false
	}
	parts := strings.SplitN(value, ".", 2)
	seconds, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, 0, false
	}
	if len(parts) == 1 {
		return seconds, 0, true
	}
	microsText := parts[1]
	if len(microsText) > 6 {
		microsText = microsText[:6]
	}
	for len(microsText) < 6 {
		microsText += "0"
	}
	micros, err := strconv.ParseInt(microsText, 10, 64)
	if err != nil {
		return 0, 0, false
	}
	return seconds, micros, true
}
