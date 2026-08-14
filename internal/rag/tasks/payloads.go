package tasks

import (
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
	"github.com/hibiken/asynq"
)

type IngestPayload struct {
	RAGSourceID   uuid.UUID  `json:"rag_source_id"`
	FromBeginning bool       `json:"from_beginning,omitempty"`
	AttemptID     *uuid.UUID `json:"attempt_id,omitempty"`
	// Entities narrows the run to a subset of the source's scope (edit reconcile).
	Entities []string `json:"entities,omitempty"`
}

type PrunePayload struct {
	RAGSourceID uuid.UUID `json:"rag_source_id"`
}

// IngestTaskID gives every source a stable queue identity. A failed task stays
// archived in Asynq, so retry can delete that exact job before enqueueing the
// replacement. The database separately guarantees at most one active attempt
// per source.
func IngestTaskID(sourceID uuid.UUID) string {
	return "rag-ingest-" + sourceID.String()
}

// IngestEnqueueOptions must also be passed to TaskEnqueuer.Enqueue. The
// enqueue client may rebuild a task to attach tracing metadata; externally
// supplied options survive that rewrite.
func IngestEnqueueOptions(sourceID uuid.UUID) []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueRagWork),
		asynq.MaxRetry(0),
		asynq.TaskID(IngestTaskID(sourceID)),
	}
}

func PruneEnqueueOptions() []asynq.Option {
	return []asynq.Option{
		asynq.Queue(QueueRagWork),
		asynq.MaxRetry(0),
	}
}

func NewIngestTask(p IngestPayload, opts ...asynq.Option) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal ingest payload: %w", err)
	}
	full := append(IngestEnqueueOptions(p.RAGSourceID), opts...)
	return asynq.NewTask(TypeRagIngest, body, full...), nil
}

func NewPruneTask(p PrunePayload, opts ...asynq.Option) (*asynq.Task, error) {
	body, err := json.Marshal(p)
	if err != nil {
		return nil, fmt.Errorf("marshal prune payload: %w", err)
	}
	full := append(PruneEnqueueOptions(), opts...)
	return asynq.NewTask(TypeRagPrune, body, full...), nil
}

func UnmarshalIngest(body []byte) (IngestPayload, error) {
	var p IngestPayload
	if err := json.Unmarshal(body, &p); err != nil {
		return IngestPayload{}, fmt.Errorf("unmarshal %s payload: %w", TypeRagIngest, err)
	}
	if p.RAGSourceID == uuid.Nil {
		return IngestPayload{}, fmt.Errorf("%s: rag_source_id required", TypeRagIngest)
	}
	return p, nil
}

func UnmarshalPrune(body []byte) (PrunePayload, error) {
	var p PrunePayload
	if err := json.Unmarshal(body, &p); err != nil {
		return PrunePayload{}, fmt.Errorf("unmarshal %s payload: %w", TypeRagPrune, err)
	}
	if p.RAGSourceID == uuid.Nil {
		return PrunePayload{}, fmt.Errorf("%s: rag_source_id required", TypeRagPrune)
	}
	return p, nil
}
