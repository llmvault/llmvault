package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/agentruntime"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

const sessionMessageLease = 5 * time.Minute

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
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
	compileDeps  agentruntime.CompileDeps
	enqueuer     enqueue.TaskEnqueuer
}

func NewSessionMessageDeliverHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator, compileDeps agentruntime.CompileDeps, enq enqueue.TaskEnqueuer) *SessionMessageDeliverHandler {
	return &SessionMessageDeliverHandler{db: db, orchestrator: orchestrator, compileDeps: compileDeps, enqueuer: enq}
}

func (h *SessionMessageDeliverHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SessionMessageDeliverPayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal payload: %w", err)
	}
	queue, err := h.claimNext(ctx, payload.SessionID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil
	}
	if err != nil {
		return err
	}
	delivery, err := h.deliverClaim(ctx, queue)
	if err != nil {
		_ = h.releaseClaim(ctx, queue.ID, err)
		return err
	}
	if err := h.markDelivered(ctx, queue.ID, delivery); err != nil {
		return err
	}
	if h.hasPending(ctx, payload.SessionID) {
		if err := EnqueueSessionMessageDeliver(ctx, h.enqueuer, payload.SessionID); err != nil {
			logging.CaptureWithFields(ctx, fmt.Errorf("enqueue next session message delivery: %w", err), map[string]any{
				"session_id": payload.SessionID.String(),
			})
		}
	}
	return nil
}

func (h *SessionMessageDeliverHandler) claimNext(ctx context.Context, sessionID uuid.UUID) (*model.SessionMessageQueue, error) {
	var claimed model.SessionMessageQueue
	err := h.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
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
			"leased_by":     "asynq",
			"leased_until":  leaseUntil,
			"last_error":    "",
		}
		if err := tx.Model(&model.SessionMessageQueue{}).Where("id = ?", row.ID).Updates(updates).Error; err != nil {
			return fmt.Errorf("mark session message processing: %w", err)
		}
		claimed = row
		claimed.Status = "processing"
		claimed.LeasedBy = "asynq"
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
	if h.orchestrator == nil {
		return nil, fmt.Errorf("session message delivery: orchestrator is required")
	}
	session := queue.Session
	event := queue.SessionEvent
	if session.ID == uuid.Nil || event.ID == uuid.Nil {
		return nil, fmt.Errorf("session message delivery: queue row missing session or event")
	}
	var agent model.Agent
	if err := h.db.WithContext(ctx).
		Where("id = ? AND org_id = ? AND status <> ?", session.AgentID, session.OrgID, "archived").
		First(&agent).Error; err != nil {
		return nil, fmt.Errorf("load session agent: %w", err)
	}
	sb, client, err := h.ensureRuntimeClient(ctx, &agent)
	if err != nil {
		return nil, err
	}
	if session.SandboxID == nil || *session.SandboxID != sb.ID {
		if err := h.db.WithContext(ctx).Model(&model.Session{}).
			Where("id = ?", session.ID).
			Update("sandbox_id", sb.ID).Error; err != nil {
			return nil, fmt.Errorf("attach session sandbox: %w", err)
		}
	}
	msg := runtimeMessageFromEvent(session.ID, event)
	resp, err := client.PostHTTPMessage(ctx, msg)
	if err != nil {
		return nil, fmt.Errorf("post session message to runtime: %w", err)
	}
	return resp, nil
}

func (h *SessionMessageDeliverHandler) ensureRuntimeClient(ctx context.Context, agent *model.Agent) (*model.Sandbox, *agentruntime.Client, error) {
	if h.compileDeps.EncKey == nil {
		return nil, nil, fmt.Errorf("session message delivery: runtime encryption key is required")
	}
	if agent == nil || agent.OrgID == nil {
		return nil, nil, fmt.Errorf("session message delivery: agent must have org_id")
	}
	sb, err := agentRuntimeSelector(h.db, h.compileDeps).MainRuntime(ctx, *agent.OrgID, agent.ID)
	if errors.Is(err, gorm.ErrRecordNotFound) {
		secrets, prepErr := agentruntime.PrepareStartup(ctx, h.compileDeps, agent)
		if prepErr != nil {
			return nil, nil, fmt.Errorf("prepare agent runtime startup: %w", prepErr)
		}
		sb, err = h.orchestrator.CreateAgentSandbox(ctx, agent, secrets)
		if err != nil {
			return nil, nil, fmt.Errorf("create agent sandbox: %w", err)
		}
		if err := agentruntime.AttachProxyTokenToSandbox(ctx, h.compileDeps, agent, sb.ID, secrets.ProxyTokenJTI); err != nil {
			return nil, nil, fmt.Errorf("tag agent proxy token sandbox: %w", err)
		}
	} else if err != nil {
		return nil, nil, fmt.Errorf("load agent sandbox: %w", err)
	}
	client, err := h.orchestrator.GetRuntimeClient(ctx, sb)
	if err != nil {
		return nil, nil, fmt.Errorf("get runtime client: %w", err)
	}
	if err := client.Readyz(ctx); err != nil {
		if err := agentruntime.PushAgentRuntimeConfig(ctx, h.compileDeps, agent, sb); err != nil {
			return nil, nil, fmt.Errorf("sync agent runtime: %w", err)
		}
		client, err = h.orchestrator.GetRuntimeClient(ctx, sb)
		if err != nil {
			return nil, nil, fmt.Errorf("get synced runtime client: %w", err)
		}
	}
	return sb, client, nil
}
