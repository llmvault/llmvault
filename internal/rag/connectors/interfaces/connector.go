package interfaces

import (
	"context"
	"encoding/json"
	"time"
)

// Source is the contract the connector layer needs from a RAGSource row.
//
// DEVIATION (from the Tranche 3B agent brief): the brief specifies
// concrete `*ragmodel.RAGSource` in factory/validator signatures.
// RAGSource lives in Tranche 3A (data model), which is executed in
// parallel to this tranche per the plan's Wave 1 launch ordering
// ("3A + 3B parallel; independent files, no shared code"). Depending
// on the concrete struct here would create a cross-tranche Go import
// that can't be satisfied until 3A merges.
//
// The Go-idiomatic solution is "interface at consumer": we declare the
// minimum behavior this layer needs from a RAGSource, and 3A's struct
// satisfies it structurally. Tranche 3C's scheduler will pass a real
// *ragmodel.RAGSource (which will trivially satisfy this interface via
// method additions or a thin adapter).
//
// The methods mirror the RAGSource fields defined in the Tranche 3A
// data-model schema.
type Source interface {
	// SourceID returns the RAGSource.ID (uuid, rendered as string).
	// Used as the lock key + attempt-attribution identifier.
	SourceID() string

	// OrgID returns the owning organization's UUID as a string.
	// Every connector query must be scoped by this.
	OrgID() string

	// SourceKind returns the connector kind ("github", "notion", ...)
	// that was used to register the factory. Matches Connector.Kind().
	SourceKind() string

	// Config returns the raw JSON config blob from RAGSource.Config.
	// Connectors parse this into their own connector-specific shape
	// during ValidateConfig.
	Config() json.RawMessage
}

// Connector is the base trait every connector implements. It identifies
// the provider kind (which matches Integration.Provider and the
// Factory registration key) and validates the per-source configuration
// at registration time — before any ingest attempt fires.
//
// We expose a deliberately minimal surface — no credential loading,
// metadata parsing, OAuth methods, or raw file callbacks. Those live
// above or below this layer in Hivy's architecture (Nango handles
// creds; metadata parsing is a pure helper; OAuth is handled at the
// Connection layer).
type Connector interface {
	// Kind returns the connector identifier (e.g. "github"). This
	// MUST match the key used at registry.Register time.
	Kind() string

	// ValidateConfig inspects src.Config() and returns an error if
	// the configuration is malformed or references unavailable
	// resources. Called at Create time (Tranche 3E) before the first
	// ingest is enqueued.
	ValidateConfig(ctx context.Context, src Source) error
}

// CheckpointedConnector yields documents in a resumable stream. It
// returns a channel that emits DocumentOrFailure items; the scheduler
// drains the channel until it closes. The connector persists its own
// state in the checkpoint T between runs — the scheduler stores the
// marshaled bytes in RAGIndexAttempt.CheckpointPointer (Phase 1B) +
// the FileStore (Phase 2B).
//
// T is constrained to Checkpoint (a marker interface) so that random
// structs can't be silently passed in. Each connector defines its own
// concrete checkpoint type (e.g. GitHubCheckpoint in Tranche 3D).
//
// The connector advances the checkpoint via its own state, and the
// scheduler reads the latest checkpoint before closing the channel.
// UnmarshalCheckpoint owns reading persisted bytes back into T.
type CheckpointedConnector[T Checkpoint] interface {
	Connector

	// LoadFromCheckpoint starts or resumes an ingest run. The returned
	// channel emits DocumentOrFailure items in no particular order; it
	// is closed when the run finishes (either naturally or on ctx
	// cancellation). The start and end bounds describe the poll
	// window; connectors may ignore them if their checkpoint already
	// pins a position.
	LoadFromCheckpoint(
		ctx context.Context,
		src Source,
		cp T,
		start, end time.Time,
	) (<-chan DocumentOrFailure, error)

	// DummyCheckpoint returns the zero-value checkpoint used for a
	// fresh "from-beginning" run.
	DummyCheckpoint() T

	// UnmarshalCheckpoint parses persisted bytes back into T. Called
	// by the scheduler (Tranche 3C) when resuming a run.
	UnmarshalCheckpoint(raw json.RawMessage) (T, error)
}

// EstimatingConnector is optional — implement only when the upstream
// can answer cheaply. Falls back to indeterminate progress when absent.
type EstimatingConnector interface {
	Connector
	EstimateTotal(ctx context.Context, src Source) (int, error)
}

// SlimConnector lists every current document ID available in the
// source. Used by the pruning loop (Tranche 3C) to detect source-side
// deletions: documents present in our index but no longer in the
// source are scheduled for delete.
type SlimConnector interface {
	Connector

	// ListAllSlim streams every current document ID.
	ListAllSlim(ctx context.Context, src Source) (<-chan SlimDocOrFailure, error)
}
