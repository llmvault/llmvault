package tasks

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

// WebhookForwardPayload is the payload for TypeWebhookForward tasks.
type WebhookForwardPayload struct {
	WebhookURL      string `json:"webhook_url"`
	EncryptedSecret []byte `json:"encrypted_secret"`
	Body            []byte `json:"body"`
}

// NewWebhookForwardTask forwards a webhook to an org's endpoint. Options are
// returned separately (not baked into the task) because the enqueue client's
// Sentry trace-payload rewrite drops baked options, demoting this critical-queue
// task to the default queue.
func NewWebhookForwardTask(webhookURL string, encryptedSecret []byte, body []byte) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(WebhookForwardPayload{
		WebhookURL:      webhookURL,
		EncryptedSecret: encryptedSecret,
		Body:            body,
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal webhook forward payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(5),
		asynq.Timeout(30 * time.Second),
	}
	return asynq.NewTask(TypeWebhookForward, payload), opts, nil
}

// EmailSendPayload is the payload for TypeEmailSend tasks.
type EmailSendPayload struct {
	To             string `json:"to"`
	Subject        string `json:"subject"`
	Body           string `json:"body"`
	IdempotencyKey string `json:"idempotency_key,omitempty"`
}

// NewEmailSendTask creates a task that sends an email. Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewEmailSendTask(to, subject, body string) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(EmailSendPayload{
		To:             to,
		Subject:        subject,
		Body:           body,
		IdempotencyKey: fmt.Sprintf("email/%s", uuid.NewString()),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal email send payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(30 * time.Second),
	}
	return asynq.NewTask(TypeEmailSend, payload), opts, nil
}

// EmailSendTemplatePayload is the payload for TypeEmailSendTemplate tasks.
// Variables is a flat string map and every value is already stringified when
// enqueued.
type EmailSendTemplatePayload struct {
	To             string            `json:"to"`
	Slug           string            `json:"slug"`
	Variables      map[string]string `json:"variables,omitempty"`
	IdempotencyKey string            `json:"idempotency_key,omitempty"`
}

// NewEmailSendTemplateTask creates a task that sends an email via a published
// transactional template resolved by slug. Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewEmailSendTemplateTask(to, slug string, variables map[string]string) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(EmailSendTemplatePayload{
		To:             to,
		Slug:           slug,
		Variables:      variables,
		IdempotencyKey: fmt.Sprintf("email-template/%s/%s", slug, uuid.NewString()),
	})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal email template payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(30 * time.Second),
	}
	return asynq.NewTask(TypeEmailSendTemplate, payload), opts, nil
}

// APIKeyUpdatePayload is the payload for TypeAPIKeyUpdate tasks.
type APIKeyUpdatePayload struct {
	KeyID uuid.UUID `json:"key_id"`
}

// NewAPIKeyUpdateTask creates a task that updates an API key's last_used_at.
// Options are returned separately (see WebhookForwardPayload's NewWebhookForwardTask).
func NewAPIKeyUpdateTask(keyID uuid.UUID) (*asynq.Task, []asynq.Option, error) {
	payload, err := json.Marshal(APIKeyUpdatePayload{KeyID: keyID})
	if err != nil {
		return nil, nil, fmt.Errorf("marshal apikey update payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueBulk),
		asynq.MaxRetry(3),
		asynq.Timeout(10 * time.Second),
	}
	return asynq.NewTask(TypeAPIKeyUpdate, payload), opts, nil
}
