package tasks

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/nango"
	"github.com/usehivy/hivy/internal/sandbox"
	"github.com/usehivy/hivy/internal/slackapp"
	"github.com/usehivy/hivy/internal/slackworkflow"
)

type SlackAppMentionHandler struct {
	db                 *gorm.DB
	orchestrator       *sandbox.Orchestrator
	compileDeps        agentruntime.CompileDeps
	enqueuer           enqueue.TaskEnqueuer
	nangoClient        slackapp.ConnectionGetter
	orgAgentEnsurer    OrgHivyAgentEnsurer
	slackClientFactory func(string) slackapp.Client
	waitFinal          func(context.Context, *agentruntime.Client, model.Session, string) (string, error)
}

func NewSlackAppMentionHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps, enq enqueue.TaskEnqueuer, nangoClient *nango.Client, ensurer OrgHivyAgentEnsurer) *SlackAppMentionHandler {
	var getter slackapp.ConnectionGetter
	if nangoClient != nil {
		getter = nangoClient
	}
	return &SlackAppMentionHandler{
		db:              db,
		orchestrator:    orchestrator,
		compileDeps:     compileDeps,
		enqueuer:        enq,
		nangoClient:     getter,
		orgAgentEnsurer: ensurer,
	}
}

func (h *SlackAppMentionHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SlackAppMentionPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal slack app mention payload: %w", err)
	}
	row, err := h.loadEvent(ctx, payload.SlackThreadEventID)
	if err != nil {
		return err
	}
	ctx = logging.WithAttrs(ctx,
		"slack_thread_event_id", row.ID.String(),
		"connection_id", row.ConnectionID.String(),
		"org_id", row.OrgID.String(),
		"slack_event_id", row.EventID,
		"slack_channel_id", row.SlackChannelID,
		"slack_thread_ts", row.ThreadTS,
	)
	if row.Status == model.SlackThreadEventStatusCompleted {
		return nil
	}
	if err := slackworkflow.MarkProcessing(ctx, h.db, row.ID); err != nil {
		return err
	}
	if err := h.process(ctx, &row); err != nil {
		slackworkflow.MarkFailed(ctx, h.db, row.ID, err)
		logging.CaptureWithFields(ctx, fmt.Errorf("slack app mention pipeline: %w", err), slackErrorFields(row, "process"))
		return err
	}
	return nil
}

func (h *SlackAppMentionHandler) process(ctx context.Context, row *model.SlackThreadEvent) error {
	token, client, err := h.slackClient(ctx, row.Connection)
	if err != nil {
		return err
	}
	statusCleared := false
	defer func() {
		if !statusCleared {
			_ = slackapp.ClearAssistantStatus(context.WithoutCancel(ctx), client, row.SlackChannelID, row.ThreadTS)
		}
	}()
	_ = slackapp.SetAssistantStatus(ctx, client, row.SlackChannelID, row.ThreadTS)

	channel, agent, err := h.resolveChannelAndAgent(ctx, row, client, token)
	if err != nil {
		return err
	}
	row.ChannelID = &channel.ID
	session, err := h.findOrCreateSlackSession(ctx, row, channel, agent)
	if err != nil {
		return err
	}
	row.SessionID = &session.ID
	intent, err := h.prepareSlackMessageIntent(ctx, row, session)
	if err != nil {
		return err
	}
	delivery, err := h.deliverSlackIntent(ctx, row, session, intent)
	if err != nil {
		return err
	}
	runtimeClient, err := h.runtimeClient(ctx, session, agent)
	if err != nil {
		return err
	}
	finalText, err := h.waitForFinalText(ctx, runtimeClient, session, delivery.TurnID)
	if err != nil {
		return err
	}
	slackworkflow.RecordFinalReceived(ctx, h.db, row.ID)
	replyTS, err := slackapp.PostThreadReply(ctx, client, row.SlackChannelID, row.ThreadTS, finalText)
	if err != nil {
		return fmt.Errorf("post slack thread reply: %w", err)
	}
	if err := h.recordSlackOutbound(ctx, row, session, finalText, replyTS); err != nil {
		logging.CaptureWithFields(ctx, err, slackErrorFields(*row, "record_outbound"))
	}
	if err := slackworkflow.RecordReplySent(ctx, h.db, row.ID, replyTS); err != nil {
		return err
	}
	if err := slackapp.ClearAssistantStatus(ctx, client, row.SlackChannelID, row.ThreadTS); err != nil {
		return fmt.Errorf("clear slack status: %w", err)
	}
	statusCleared = true
	logging.FromContext(ctx).InfoContext(ctx, "slack_app_mention_completed",
		"slack_thread_event_id", row.ID.String(),
		"session_id", session.ID.String(),
		"slack_reply_ts", replyTS,
		"runtime_turn_id", delivery.TurnID,
	)
	return nil
}

func (h *SlackAppMentionHandler) loadEvent(ctx context.Context, id uuid.UUID) (model.SlackThreadEvent, error) {
	var row model.SlackThreadEvent
	err := h.db.WithContext(ctx).
		Preload("Connection.Integration").
		First(&row, "id = ?", id).Error
	if err != nil {
		return model.SlackThreadEvent{}, fmt.Errorf("load slack thread event: %w", err)
	}
	return row, nil
}

func (h *SlackAppMentionHandler) slackClient(ctx context.Context, conn model.Connection) (string, slackapp.Client, error) {
	token, err := slackapp.LoadBotToken(ctx, h.nangoClient, conn)
	if err != nil {
		return "", nil, err
	}
	if h.slackClientFactory != nil {
		return token, h.slackClientFactory(token), nil
	}
	return token, slackapp.NewClient(token), nil
}

func slackErrorFields(row model.SlackThreadEvent, stage string) map[string]any {
	return map[string]any{
		"stage":                 stage,
		"slack_thread_event_id": row.ID.String(),
		"org_id":                row.OrgID.String(),
		"connection_id":         row.ConnectionID.String(),
		"session_id":            uuidPtrString(row.SessionID),
		"event_id":              row.EventID,
		"event_type":            row.EventType,
		"slack_channel_id":      row.SlackChannelID,
		"slack_thread_ts":       row.ThreadTS,
		"slack_message_ts":      row.MessageTS,
	}
}

func uuidPtrString(id *uuid.UUID) string {
	if id == nil || *id == uuid.Nil {
		return ""
	}
	return id.String()
}
