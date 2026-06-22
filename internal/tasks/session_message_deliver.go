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
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const sessionMessageLease = 5 * time.Minute

var (
	ErrSessionTurnActive      = errors.New("session agent turn active")
	ErrSessionRuntimeNotReady = errors.New("session runtime not ready")
	ErrSessionRuntimeDraining = errors.New("session runtime draining")
)

func init() {
	RegisterTaskBuilder(TypeSessionMessageDeliver, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p SessionMessageDeliverPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal session message deliver payload: %w", err)
		}
		return NewSessionMessageDeliverTask(p)
	})
}

type SessionMessageDeliverPayload struct {
	SessionID uuid.UUID `json:"session_id"`
}

type SessionMessageCommand struct {
	ActorUserID     *uuid.UUID
	Text            string
	Payload         model.JSON
	Model           string
	ReasoningEffort string
}

func NewSessionMessageDeliverTask(payload SessionMessageDeliverPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.SessionID == uuid.Nil {
		return nil, nil, fmt.Errorf("session_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal session message deliver payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueCritical),
		asynq.MaxRetry(5),
		asynq.Timeout(3 * time.Minute),
	}
	return asynq.NewTask(TypeSessionMessageDeliver, encoded), opts, nil
}

func EnqueueSessionMessageDeliver(ctx context.Context, enq enqueue.TaskEnqueuer, sessionID uuid.UUID) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewSessionMessageDeliverTask(SessionMessageDeliverPayload{SessionID: sessionID})
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		return err
	}
	return nil
}

type SessionMessageDeliverHandler struct {
	db                *gorm.DB
	orchestrator      *sandbox.Orchestrator
	compileDeps       agentruntime.CompileDeps
	enqueuer          enqueue.TaskEnqueuer
	allowProvisioning bool
	leaseOwner        string
}

func NewSessionMessageDeliverHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps, enq enqueue.TaskEnqueuer) *SessionMessageDeliverHandler {
	return &SessionMessageDeliverHandler{
		db:                db,
		orchestrator:      orchestrator,
		compileDeps:       compileDeps,
		enqueuer:          enq,
		allowProvisioning: true,
		leaseOwner:        "asynq",
	}
}

func (h *SessionMessageDeliverHandler) WithoutProvisioning() *SessionMessageDeliverHandler {
	if h == nil {
		return nil
	}
	copy := *h
	copy.allowProvisioning = false
	copy.leaseOwner = "api"
	return &copy
}

func (h *SessionMessageDeliverHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SessionMessageDeliverPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	if _, err := h.DispatchNext(ctx, payload.SessionID); errors.Is(err, gorm.ErrRecordNotFound) || errors.Is(err, ErrSessionTurnActive) || errors.Is(err, ErrSessionRuntimeDraining) {
		return nil
	} else if err != nil {
		return err
	}
	return nil
}

func (h *SessionMessageDeliverHandler) DispatchNext(ctx context.Context, sessionID uuid.UUID) (*agentruntime.HTTPMessageResponse, error) {
	queue, err := h.claimNext(ctx, sessionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}
	if err != nil {
		return nil, err
	}
	delivery, err := h.deliverClaim(ctx, queue)
	if err != nil {
		_ = h.releaseClaim(ctx, queue.ID, err)
		if errors.Is(err, ErrSessionRuntimeDraining) {
			_ = h.releaseSessionTurnIdle(ctx, queue.SessionID)
		} else {
			_ = h.releaseSessionTurn(ctx, queue.SessionID)
		}
		return nil, err
	}
	if err := h.markDelivered(ctx, queue, delivery); err != nil {
		return nil, err
	}
	return delivery, nil
}

func (h *SessionMessageDeliverHandler) claimNext(ctx context.Context, sessionID uuid.UUID) (*model.SessionMessageQueue, error) {
	var claimed model.SessionMessageQueue
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var session model.Session
		if err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("id = ?", sessionID).
			First(&session).Error; err != nil {
			return fmt.Errorf("lock session for message delivery: %w", err)
		}
		if session.AgentTurnStatus == model.SessionAgentTurnActive {
			return ErrSessionTurnActive
		}
		draining, err := sessionRuntimeDraining(ctx, tx, session)
		if err != nil {
			return fmt.Errorf("check session runtime drain status: %w", err)
		}
		if draining {
			return ErrSessionRuntimeDraining
		}
		var row model.SessionMessageQueue
		res := tx.Raw(`
SELECT q.*
FROM session_message_queue q
WHERE q.session_id = ?
  AND (q.status = 'pending' OR (q.status = 'processing' AND (q.leased_until IS NULL OR q.leased_until < now())))
  AND NOT EXISTS (
    SELECT 1
    FROM session_message_queue prev
    WHERE prev.session_id = q.session_id
      AND prev.sequence_number < q.sequence_number
      AND prev.status <> 'delivered'
  )
ORDER BY q.sequence_number ASC
LIMIT 1
FOR UPDATE SKIP LOCKED`, sessionID).Scan(&row)
		if res.Error != nil {
			return fmt.Errorf("claim session message row: %w", res.Error)
		}
		if res.RowsAffected == 0 {
			return gorm.ErrRecordNotFound
		}
		leaseUntil := time.Now().Add(sessionMessageLease)
		updates := map[string]any{
			"status":        "processing",
			"attempt_count": gorm.Expr("attempt_count + 1"),
			"leased_by":     h.leaseOwner,
			"leased_until":  leaseUntil,
			"last_error":    "",
		}
		if err := tx.Model(&model.SessionMessageQueue{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("mark session message processing: %w", err)
		}
		if err := tx.Model(&model.Session{}).
			Where("id = ?", sessionID).
			Updates(map[string]any{
				"agent_turn_status":     model.SessionAgentTurnActive,
				"agent_turn_id":         "",
				"agent_stream_id":       "",
				"agent_turn_started_at": time.Now(),
			}).Error; err != nil {
			return fmt.Errorf("mark session agent turn active: %w", err)
		}
		claimed = row
		claimed.Status = "processing"
		claimed.LeasedBy = h.leaseOwner
		claimed.LeasedUntil = &leaseUntil
		return nil
	})
	if err != nil {
		return nil, err
	}
	if err := h.db.WithContext(ctx).
		Preload("Session").
		Preload("SessionEvent").
		First(&claimed, "id = ?", claimed.ID).Error; err != nil {
		return nil, fmt.Errorf("load claimed session message: %w", err)
	}
	return &claimed, nil
}

func (h *SessionMessageDeliverHandler) deliverClaim(ctx context.Context, queue *model.SessionMessageQueue) (*agentruntime.HTTPMessageResponse, error) {
	session := queue.Session
	if session.ID == uuid.Nil {
		return nil, fmt.Errorf("session message delivery: queue row missing session")
	}
	command := SessionMessageCommand{
		ActorUserID:     queue.ActorUserID,
		Text:            queue.MessageText,
		Payload:         queue.MessagePayload,
		Model:           queue.Model,
		ReasoningEffort: queue.ReasoningEffort,
	}
	if strings.TrimSpace(command.Text) == "" && queue.SessionEvent != nil {
		command = commandFromLegacyEvent(*queue.SessionEvent)
	}
	return h.DeliverCommand(ctx, session, command)
}

// DeliverEvent posts a persisted session event directly to the runtime without
// creating or updating any queue rows.
func (h *SessionMessageDeliverHandler) DeliverEvent(ctx context.Context, session model.Session, event model.SessionEvent) (*agentruntime.HTTPMessageResponse, error) {
	return h.DeliverCommand(ctx, session, commandFromLegacyEvent(event))
}

// DeliverCommand posts a user command directly to the runtime without
// creating or updating any queue rows.
func (h *SessionMessageDeliverHandler) DeliverCommand(ctx context.Context, session model.Session, command SessionMessageCommand) (*agentruntime.HTTPMessageResponse, error) {
	if h.orchestrator == nil {
		return nil, fmt.Errorf("session message delivery: orchestrator is required")
	}
	if session.ID == uuid.Nil {
		return nil, fmt.Errorf("session message delivery: session is required")
	}
	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", session.AgentID, session.OrgID, "archived").
		First(&agent).Error; err != nil {
		return nil, fmt.Errorf("load session agent: %w", err)
	}
	sb, client, err := h.ensureRuntimeClient(ctx, session, &agent)
	if err != nil {
		return nil, err
	}
	if session.SandboxID == nil || *session.SandboxID != sb.ID {
		if err := h.db.WithContext(ctx).Model(&model.Session{}).
			Where("id = ?", session.ID).
			Update("sandbox_id", sb.ID).Error; err != nil {
			return nil, fmt.Errorf("attach session sandbox: %w", err)
		}
		session.SandboxID = &sb.ID
	}
	modelName := firstNonEmpty(command.Model, session.Model)
	reasoningEffort := firstNonEmpty(command.ReasoningEffort, session.ReasoningEffort)
	modelDef, err := agentruntime.SessionModelDefinition(ctx, h.compileDeps, &agent, modelName, reasoningEffort)
	if err != nil {
		return nil, fmt.Errorf("build session model definition: %w", err)
	}
	msg := runtimeMessageFromCommand(session, command, modelDef)
	resp, err := client.PostHTTPMessage(ctx, msg)
	if err != nil {
		if agentruntime.IsRuntimeDrainingError(err) {
			return nil, ErrSessionRuntimeDraining
		}
		return nil, fmt.Errorf("post session message to runtime: %w", err)
	}
	return resp, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
