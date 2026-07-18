package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentemail"
	"github.com/usehivy/hivy/internal/model"
)

func init() {
	RegisterTaskBuilder(TypeAgentEmailSend, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p AgentEmailSendPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal agent email send payload: %w", err)
		}
		return NewAgentEmailSendTask(p)
	})
}

type AgentEmailSendPayload struct {
	MessageID uuid.UUID `json:"message_id"`
}

func NewAgentEmailSendTask(payload AgentEmailSendPayload) (*asynq.Task, []asynq.Option, error) {
	return agentemail.NewSendTask(payload.MessageID)
}

type AgentEmailSendHandler struct {
	db     *gorm.DB
	client *agentemail.Client
	domain string
}

func NewAgentEmailSendHandler(db *gorm.DB, client *agentemail.Client, domain string) *AgentEmailSendHandler {
	return &AgentEmailSendHandler{db: db, client: client, domain: strings.ToLower(strings.TrimSpace(domain))}
}

func (h *AgentEmailSendHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload AgentEmailSendPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal agent email send task: %w", err)
	}
	if h.domain == "" {
		return fmt.Errorf("agent inbox domain is not configured")
	}
	var message model.AgentEmailMessage
	if err := h.db.WithContext(ctx).Where("id = ? AND direction = ?", payload.MessageID, model.AgentEmailDirectionOutbound).First(&message).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		return fmt.Errorf("load outgoing agent email: %w", err)
	}
	if message.Status == model.AgentEmailStatusSent {
		return nil
	}
	var agent model.Agent
	if err := h.db.WithContext(ctx).Where("id = ? AND org_id = ? AND status <> ?", message.AgentID, message.OrgID, "archived").First(&agent).Error; err != nil {
		return fmt.Errorf("load sending agent: %w", err)
	}
	var thread model.AgentEmailThread
	if err := h.db.WithContext(ctx).Where("id = ? AND agent_id = ? AND org_id = ?", message.ThreadID, agent.ID, message.OrgID).First(&thread).Error; err != nil {
		return fmt.Errorf("load outgoing email thread: %w", err)
	}
	var to, cc []string
	if err := json.Unmarshal(message.ToAddresses, &to); err != nil {
		return fmt.Errorf("decode outgoing email recipients: %w", err)
	}
	if err := json.Unmarshal(message.CCAddresses, &cc); err != nil {
		return fmt.Errorf("decode outgoing email cc recipients: %w", err)
	}
	headers := map[string]string{}
	var lastInbound model.AgentEmailMessage
	if err := h.db.WithContext(ctx).Where("thread_id = ? AND direction = ? AND message_id <> ?", thread.ID, model.AgentEmailDirectionInbound, "").Order("provider_at DESC").First(&lastInbound).Error; err == nil {
		headers["In-Reply-To"] = lastInbound.MessageID
		var refs []string
		if err := json.Unmarshal(lastInbound.References, &refs); err != nil {
			return fmt.Errorf("decode email thread references: %w", err)
		}
		refs = append(refs, lastInbound.MessageID)
		if len(refs) > 20 {
			refs = refs[len(refs)-20:]
		}
		if len(refs) > 0 {
			headers["References"] = strings.Join(refs, " ")
		}
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("load email thread reply target: %w", err)
	}
	from := agent.EmailInboxLocalPart + "@" + h.domain
	result, err := h.client.Send(ctx, agentemail.SendRequest{From: from, To: to, CC: cc, Subject: message.Subject, Text: message.TextBody, HTML: message.HTMLBody, Headers: headers, ReplyTo: agentemail.ReplyLocalPart(thread.ReplyToken) + "@" + h.domain}, "agent-email/"+message.ID.String())
	if err != nil {
		return err
	}
	now := time.Now().UTC()
	if err := h.db.WithContext(ctx).Model(&model.AgentEmailMessage{}).Where("id = ? AND status = ?", message.ID, model.AgentEmailStatusQueued).Updates(map[string]any{"status": model.AgentEmailStatusSent, "resend_email_id": result.ID, "provider_at": now, "headers": stringMapJSON(headers)}).Error; err != nil {
		return fmt.Errorf("mark outgoing email sent: %w", err)
	}
	if err := h.db.WithContext(ctx).Model(&model.AgentEmailThread{}).Where("id = ?", thread.ID).Update("last_message_at", now).Error; err != nil {
		return fmt.Errorf("update outgoing email thread activity: %w", err)
	}
	return nil
}
