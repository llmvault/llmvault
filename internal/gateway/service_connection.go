package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/precontext"
)

type ReceiveConnectionResult struct {
	Inbound               InboundEnvelope
	Session               model.AgentSession
	RuntimeConversationID string
	RuntimeSessionID      string
	StreamURL             string
	ResponseStreamURL     string
	RuntimeURL            string
	RuntimeAPIKey         string
	TraceID               string
	TurnID                string
	ActionToken           string
	Duplicate             bool
	Ignored               bool
	IgnoreReason          string
}

func (s *Service) ReceiveWebhookFromConnection(ctx context.Context, envelope WebhookEnvelope) (*ReceiveConnectionResult, error) {
	if s.runtime == nil {
		return nil, fmt.Errorf("gateway runtime messenger is not configured")
	}

	adapter, err := s.adapter(envelope.Provider)
	if err != nil {
		return nil, err
	}

	inbound, ok, err := adapter.DecodeInbound(ctx, envelope)
	if err != nil {
		return nil, fmt.Errorf("decode inbound: %w", err)
	}
	if !ok {
		return &ReceiveConnectionResult{Ignored: true, IgnoreReason: "adapter_ignored"}, nil
	}
	inbound.Provider = envelope.Provider

	if strings.TrimSpace(inbound.ThreadKey) == "" {
		return nil, fmt.Errorf("gateway inbound thread key is required")
	}
	if strings.TrimSpace(inbound.DedupeKey) == "" {
		inbound.DedupeKey = inbound.ExternalMessageID
	}
	if strings.TrimSpace(inbound.DedupeKey) == "" {
		return nil, fmt.Errorf("gateway inbound dedupe key is required")
	}
	if inbound.ReceivedAt.IsZero() {
		inbound.ReceivedAt = s.now().UTC()
	}

	decision, err := s.allowConnectionInbound(ctx, envelope, inbound)
	if err != nil {
		return nil, err
	}
	if !decision.allowed {
		return &ReceiveConnectionResult{Inbound: inbound, Ignored: true, IgnoreReason: decision.reason}, nil
	}

	route := model.AgentGatewayRoute{
		ID:       uuid.Nil,
		OrgID:    envelope.OrgID,
		AgentID:  envelope.AgentID,
		Provider: envelope.Provider,
	}

	event, duplicate, err := s.insertInboundEvent(ctx, route, inbound)
	if err != nil {
		return nil, err
	}
	if duplicate {
		return &ReceiveConnectionResult{Inbound: inbound, Duplicate: true, IgnoreReason: "duplicate_event"}, nil
	}
	if s.onConnectionInboundAccepted != nil {
		s.onConnectionInboundAccepted(ctx, ConnectionInboundAccepted{
			Envelope: envelope,
			Inbound:  inbound,
			Event:    event,
		})
	}

	req, err := adapter.FormatAgentRequest(ctx, inbound)
	if err != nil {
		_ = s.markEventFailed(ctx, event.ID, err)
		return nil, err
	}

	session, conversationID, created, err := s.findOrCreateSessionByConnection(ctx, envelope, inbound.ThreadKey)
	if err != nil {
		_ = s.markEventFailed(ctx, event.ID, err)
		return nil, err
	}
	if created {
		s.notifySessionCreated(ctx, session, "gateway_session_created", "gateway.session.created")
	}
	dynamicContext := s.buildPreContext(ctx, created, precontext.Request{
		OrgID:                 envelope.OrgID,
		AgentID:               envelope.AgentID,
		CurrentSessionID:      session.ID,
		RuntimeConversationID: conversationID,
		Text:                  req.Markdown,
		UserID:                inbound.SenderID,
		UserDisplayName:       inbound.SenderName,
		Source:                envelope.Provider,
	})

	delivery, err := s.runtime.Send(ctx, RuntimeMessage{
		Session:              session,
		Text:                 req.Markdown,
		User:                 inbound.SenderID,
		UserDisplayName:      inbound.SenderName,
		ConversationID:       conversationID,
		GatewayEventID:       event.ID,
		GatewayDedupeKey:     inbound.DedupeKey,
		GatewayThreadKey:     inbound.ThreadKey,
		GatewayChannelID:     inbound.ChannelID,
		GatewayThreadID:      inbound.ThreadID,
		GatewayExternalMsgID: inbound.ExternalMessageID,
		GatewayProvider:      envelope.Provider,
		DynamicContext:       dynamicContext,
		Metadata:             req.Metadata,
	})
	if err != nil {
		_ = s.markEventFailed(ctx, event.ID, err)
		return nil, err
	}
	if delivery == nil {
		delivery = &RuntimeDelivery{}
	}

	if err := s.markEventDelivered(ctx, event.ID, session.ID, conversationID, delivery); err != nil {
		return nil, err
	}

	var sandbox model.Sandbox
	if err := s.db.WithContext(ctx).Where("id = ?", session.SandboxID).First(&sandbox).Error; err != nil {
		return nil, fmt.Errorf("load sandbox: %w", err)
	}

	runtimeAPIKey := ""
	if s.encKey != nil && len(sandbox.EncryptedRuntimeSecret) > 0 {
		decrypted, err := s.encKey.DecryptString(sandbox.EncryptedRuntimeSecret)
		if err != nil {
			return nil, fmt.Errorf("decrypt runtime secret: %w", err)
		}
		runtimeAPIKey = decrypted
	}

	actionToken := ""
	if slackAdapter, ok := adapter.(*SlackAdapter); ok {
		actionToken = slackAdapter.ActionToken(inbound.Raw)
	}

	runtimeURL := ""
	if sandbox.RuntimeURL != "" {
		runtimeURL = sandbox.RuntimeURL
	}
	responseStreamURL := absoluteRuntimeURL(runtimeURL, delivery.ResponseStreamURL)
	if responseStreamURL == "" && delivery.ResponseStreamID != "" {
		responseStreamURL = runtimeURL + "/gateway/http/response-streams/" + delivery.ResponseStreamID
	}

	return &ReceiveConnectionResult{
		Inbound:               inbound,
		Session:               session,
		RuntimeConversationID: conversationID,
		RuntimeSessionID:      delivery.SessionID,
		StreamURL:             runtimeURL + "/gateway/http/streams/" + delivery.StreamID,
		ResponseStreamURL:     responseStreamURL,
		RuntimeURL:            runtimeURL,
		RuntimeAPIKey:         runtimeAPIKey,
		TraceID:               delivery.TraceID,
		TurnID:                delivery.TurnID,
		ActionToken:           actionToken,
	}, nil
}

func absoluteRuntimeURL(runtimeURL, pathOrURL string) string {
	if strings.TrimSpace(pathOrURL) == "" {
		return ""
	}
	if strings.HasPrefix(pathOrURL, "http://") || strings.HasPrefix(pathOrURL, "https://") {
		return pathOrURL
	}
	return strings.TrimRight(runtimeURL, "/") + "/" + strings.TrimLeft(pathOrURL, "/")
}

func (s *Service) findOrCreateSessionByConnection(ctx context.Context, envelope WebhookEnvelope, threadKey string) (model.AgentSession, string, bool, error) {
	conversationID := stableConversationID(envelope.ConnectionID, threadKey)
	sessionID := runtimeSessionID(conversationID)

	sandbox, err := s.agentSandboxSelector().MainRuntime(ctx, envelope.OrgID, envelope.AgentID)
	if err != nil {
		return model.AgentSession{}, "", false, fmt.Errorf("load agent sandbox: %w", err)
	}

	connectionID := envelope.ConnectionID
	session := model.AgentSession{}
	created := false
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		err := tx.Where("org_id = ? AND agent_id = ? AND source = ? AND source_id = ? AND source_resource_key = ? AND status = ?",
			envelope.OrgID, envelope.AgentID, Source, envelope.ConnectionID, threadKey, "active").
			First(&session).Error
		if err == nil {
			if session.SandboxID != sandbox.ID {
				if err := tx.Model(&session).Update("sandbox_id", sandbox.ID).Error; err != nil {
					return err
				}
				session.SandboxID = sandbox.ID
			}
			return nil
		}
		if !errors.Is(err, gorm.ErrRecordNotFound) {
			return err
		}
		session = model.AgentSession{
			OrgID:                 envelope.OrgID,
			AgentID:               envelope.AgentID,
			SandboxID:             sandbox.ID,
			RuntimeConversationID: sessionID,
			Source:                Source,
			SourceID:              &connectionID,
			SourceResourceKey:     threadKey,
			Status:                "active",
			Name:                  "Gateway: " + threadKey,
			IntegrationScopes:     model.JSON{},
		}
		if err := tx.Create(&session).Error; err != nil {
			return err
		}
		created = true
		return nil
	})
	if err != nil {
		return model.AgentSession{}, "", false, fmt.Errorf("find or create gateway session: %w", err)
	}
	return session, conversationID, created, nil
}

func (s *Service) loadConnection(ctx context.Context, id uuid.UUID) (*model.Connection, error) {
	var conn model.Connection
	if err := s.db.WithContext(ctx).
		Preload("Integration").
		Where("id = ? AND revoked_at IS NULL", id).
		First(&conn).Error; err != nil {
		return nil, fmt.Errorf("load connection: %w", err)
	}
	return &conn, nil
}
