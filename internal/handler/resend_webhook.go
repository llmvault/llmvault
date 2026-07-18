package handler

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"

	svix "github.com/svix/svix-webhooks/go"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/tasks"
)

const maxResendWebhookBytes = 1 << 20

// ResendWebhookHandler receives every Resend event at one endpoint. Resend
// sends only metadata for email.received; the worker retrieves the body later.
type ResendWebhookHandler struct {
	db       *gorm.DB
	secret   string
	domain   string
	enqueuer enqueue.TaskEnqueuer
}

func NewResendWebhookHandler(db *gorm.DB, secret, domain string, enqueuer enqueue.TaskEnqueuer) *ResendWebhookHandler {
	return &ResendWebhookHandler{db: db, secret: strings.TrimSpace(secret), domain: strings.TrimSpace(domain), enqueuer: enqueuer}
}

type resendWebhookEvent struct {
	Type string `json:"type"`
	Data struct {
		EmailID string `json:"email_id"`
	} `json:"data"`
}

// Handle processes the configured Resend webhook path. The raw body is
// verified before JSON parsing because Svix signatures cover the original bytes.
func (h *ResendWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	if h == nil || h.db == nil || h.enqueuer == nil || h.secret == "" || h.domain == "" {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "email receiving is not configured"})
		return
	}
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, maxResendWebhookBytes))
	if err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook body"})
		return
	}
	webhook, err := svix.NewWebhook(h.secret)
	if err != nil || webhook.Verify(body, r.Header) != nil {
		logging.FromContext(r.Context()).WarnContext(r.Context(), "resend webhook signature rejected")
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "invalid webhook signature"})
		return
	}
	var event resendWebhookEvent
	if err := json.Unmarshal(body, &event); err != nil || strings.TrimSpace(event.Type) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook payload"})
		return
	}
	if event.Type != "email.received" {
		writeJSON(w, http.StatusOK, statusResponse{Status: "ignored"})
		return
	}
	svixID := strings.TrimSpace(r.Header.Get("svix-id"))
	if svixID == "" || strings.TrimSpace(event.Data.EmailID) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid webhook payload"})
		return
	}
	receipt := model.AgentEmailWebhookReceipt{
		SvixID: svixID, EventType: event.Type, ResendEmailID: event.Data.EmailID, Payload: model.RawJSON(body),
	}
	result := h.db.WithContext(r.Context()).Clauses(clause.OnConflict{DoNothing: true}).Create(&receipt)
	if result.Error != nil {
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "store resend webhook receipt", "error", result.Error)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to receive email"})
		return
	}
	task, opts, err := tasks.NewAgentEmailReceiveTask(tasks.AgentEmailReceivePayload{SvixID: svixID})
	if err == nil {
		_, err = h.enqueuer.EnqueueContext(r.Context(), task, opts...)
	}
	if err != nil {
		// Return non-2xx: Resend retries, while the receipt makes the retry safe.
		logging.FromContext(r.Context()).ErrorContext(r.Context(), "enqueue Resend email processing", "error", err)
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to receive email"})
		return
	}
	writeJSON(w, http.StatusOK, statusResponse{Status: "received"})
}
