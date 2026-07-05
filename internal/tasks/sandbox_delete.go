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

	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/sandbox"
)

// SandboxDeletePayload identifies a single sandbox to tear down once its
// session has been archived.
type SandboxDeletePayload struct {
	SandboxID uuid.UUID `json:"sandbox_id"`
}

// NewSandboxDeleteTask builds a sandbox:delete task. Options are returned
// separately (see NewSandboxMarkRunningTask).
func NewSandboxDeleteTask(payload SandboxDeletePayload) (*asynq.Task, []asynq.Option, error) {
	if payload.SandboxID == uuid.Nil {
		return nil, nil, fmt.Errorf("sandbox_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal sandbox delete payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(2 * time.Minute),
		// Collapse repeat archives of the same sandbox into a single teardown.
		asynq.Unique(2 * time.Minute),
	}
	return asynq.NewTask(TypeSandboxDelete, encoded), opts, nil
}

// EnqueueSandboxDelete schedules teardown of a session's sandbox after the
// session is archived. Safe to call with a nil enqueuer or a nil sandbox ID
// (both no-op).
func EnqueueSandboxDelete(ctx context.Context, enq enqueue.TaskEnqueuer, sandboxID uuid.UUID) error {
	if enq == nil || sandboxID == uuid.Nil {
		return nil
	}
	task, opts, err := NewSandboxDeleteTask(SandboxDeletePayload{SandboxID: sandboxID})
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		return err
	}
	return nil
}

// SandboxDeleteHandler tears down the provider compute for a single sandbox
// whose session has been archived, keeping the sandbox row as an 'archived'
// record for history.
type SandboxDeleteHandler struct {
	db           *gorm.DB
	orchestrator *sandbox.Orchestrator
}

// NewSandboxDeleteHandler creates a new sandbox delete handler.
func NewSandboxDeleteHandler(db *gorm.DB, orchestrator *sandbox.Orchestrator) *SandboxDeleteHandler {
	return &SandboxDeleteHandler{db: db, orchestrator: orchestrator}
}

// Handle processes a sandbox:delete task.
func (h *SandboxDeleteHandler) Handle(ctx context.Context, task *asynq.Task) error {
	var payload SandboxDeletePayload
	if err := json.Unmarshal(task.Payload(), &payload); err != nil {
		return fmt.Errorf("unmarshal sandbox delete payload: %w", err)
	}
	if h.orchestrator == nil {
		return nil
	}

	var sb model.Sandbox
	if err := h.db.WithContext(ctx).First(&sb, "id = ?", payload.SandboxID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil // already gone
		}
		return fmt.Errorf("load sandbox %s: %w", payload.SandboxID, err)
	}

	// Already torn down (row kept as archived): only ensure credentials are dead.
	if sb.Status == string(sandbox.StatusArchived) {
		h.revokeProxyTokens(ctx, sb.ID)
		return nil
	}

	// Frees provider compute but keeps the DB row marked 'archived'.
	if err := h.orchestrator.DeleteSandboxResource(ctx, &sb); err != nil {
		return fmt.Errorf("delete sandbox resource %s: %w", sb.ID, err)
	}
	h.revokeProxyTokens(ctx, sb.ID)
	logging.FromContext(ctx).InfoContext(ctx, "sandbox delete complete", "sandbox_id", sb.ID)
	return nil
}

// revokeProxyTokens kills every non-revoked proxy token minted for the sandbox
// so a torn-down sandbox cannot keep using its LLM/MCP credentials.
func (h *SandboxDeleteHandler) revokeProxyTokens(ctx context.Context, sandboxID uuid.UUID) {
	now := time.Now()
	if err := h.db.WithContext(ctx).Model(&model.Token{}).
		Where("meta ->> ? = ? AND revoked_at IS NULL", model.TokenMetaSandboxID, sandboxID.String()).
		Update("revoked_at", &now).Error; err != nil {
		logging.Capture(ctx, fmt.Errorf("revoke proxy tokens for sandbox %s: %w", sandboxID, err))
	}
}
