package tasks

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"

	"github.com/usehivy/hivy/internal/enqueue"
)

// MemoryConsolidatePayload keys one consolidation run: all unconsolidated
// reflection facts of one agent in one org.
type MemoryConsolidatePayload struct {
	OrgID   uuid.UUID `json:"org_id"`
	AgentID uuid.UUID `json:"agent_id"`
}

// ObservationEmbedPayload retries the embedding of one observation revision
// when the consolidation worker's synchronous embed failed.
type ObservationEmbedPayload struct {
	ObservationID uuid.UUID `json:"observation_id"`
	Revision      int       `json:"revision"`
}

func init() {
	RegisterTaskBuilder(TypeMemoryConsolidate, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p MemoryConsolidatePayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal memory consolidate payload: %w", err)
		}
		return NewMemoryConsolidateTask(p)
	})
	RegisterTaskBuilder(TypeObservationEmbed, func(payload []byte) (*asynq.Task, []asynq.Option, error) {
		var p ObservationEmbedPayload
		if err := json.Unmarshal(payload, &p); err != nil {
			return nil, nil, fmt.Errorf("unmarshal observation embed payload: %w", err)
		}
		return NewObservationEmbedTask(p)
	})
}

func NewMemoryConsolidateTask(payload MemoryConsolidatePayload) (*asynq.Task, []asynq.Option, error) {
	if payload.OrgID == uuid.Nil {
		return nil, nil, fmt.Errorf("org_id is required")
	}
	if payload.AgentID == uuid.Nil {
		return nil, nil, fmt.Errorf("agent_id is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal memory consolidate payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(3),
		asynq.Timeout(4 * time.Minute),
		// Uniqueness is payload-keyed, so one pending run per org+agent.
		asynq.Unique(time.Minute),
	}
	return asynq.NewTask(TypeMemoryConsolidate, encoded), opts, nil
}

// EnqueueMemoryConsolidate schedules a consolidation run for one agent's
// unconsolidated reflection facts. Called right after each reflection run
// stores facts, and by the periodic stranded-facts sweep. Duplicate enqueues
// within the uniqueness window collapse into one run.
func EnqueueMemoryConsolidate(ctx context.Context, enq enqueue.TaskEnqueuer, orgID, agentID uuid.UUID) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewMemoryConsolidateTask(MemoryConsolidatePayload{OrgID: orgID, AgentID: agentID})
	if err != nil {
		return err
	}
	if _, err := enq.EnqueueContext(ctx, task, opts...); err != nil {
		if errors.Is(err, asynq.ErrDuplicateTask) || errors.Is(err, asynq.ErrTaskIDConflict) {
			return nil
		}
		return err
	}
	return nil
}

func NewObservationEmbedTask(payload ObservationEmbedPayload) (*asynq.Task, []asynq.Option, error) {
	if payload.ObservationID == uuid.Nil {
		return nil, nil, fmt.Errorf("observation_id is required")
	}
	if payload.Revision <= 0 {
		return nil, nil, fmt.Errorf("revision is required")
	}
	encoded, err := json.Marshal(payload)
	if err != nil {
		return nil, nil, fmt.Errorf("marshal observation embed payload: %w", err)
	}
	opts := []asynq.Option{
		asynq.Queue(QueueDefault),
		asynq.MaxRetry(5),
		asynq.Timeout(2 * time.Minute),
	}
	return asynq.NewTask(TypeObservationEmbed, encoded), opts, nil
}

// EnqueueObservationEmbed retries an observation embedding asynchronously.
func EnqueueObservationEmbed(ctx context.Context, enq enqueueTaskEnqueuer, observationID uuid.UUID, revision int) error {
	if enq == nil {
		return nil
	}
	task, opts, err := NewObservationEmbedTask(ObservationEmbedPayload{ObservationID: observationID, Revision: revision})
	if err != nil {
		return err
	}
	_, err = enq.EnqueueContext(ctx, task, opts...)
	return err
}
