package model

import (
	"time"

	"github.com/google/uuid"
)

// RAGIndexAttempt represents one attempted ingestion pass of documents
// from a connected source (think: one Google Drive pull, one GitHub
// crawl). Each attempt is scoped to a single (Connection,
// EmbeddingModel) pair because the one-model-per-org invariant means
// switching the model = starting a new attempt against a new dataset.
//
// Notes:
//   - PK is a uuid (not an autoincrement int). Hivy convention —
//     every table uses uuid.UUID PKs.
//   - Every RAG table keys off the top-level RAGSource, so this
//     attempt is scoped to `rag_source_id`.
//   - EmbeddingModelID is an FK to `rag_embedding_models(id)`. The FK
//     is installed by the model package's Migrate entry point after
//     the embedding-model catalog has been created.
//   - OrgID carries row-level tenancy with a CASCADE FK to orgs(id).
type RAGIndexAttempt struct {
	// ID is the primary key.
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	// OrgID enables row-level tenancy.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`

	// RAGSourceID — FK to rag_sources(id) with CASCADE so that
	// tearing down a source wipes its index-attempt history.
	// RAGSource carries the Connection reference for
	// INTEGRATION-kind sources.
	RAGSourceID uuid.UUID `gorm:"type:uuid;not null;index"`

	// EmbeddingModelID is an FK to rag_embedding_models(id).
	EmbeddingModelID *string `gorm:"type:text"`

	// FromBeginning — only set when the attempt was kicked off via
	// the run-once API with reindex-from-zero semantics.
	FromBeginning bool `gorm:"not null;default:false"`

	// Status is the lifecycle status of this attempt.
	Status IndexingStatus `gorm:"type:text;not null;index"`

	// Document counters. Nullable because an attempt that hasn't
	// started reporting progress yet has no meaningful value (a 0
	// would be indistinguishable from "zero docs processed so far").
	NewDocsIndexed       *int `gorm:"default:0"`
	TotalDocsIndexed     *int `gorm:"default:0"`
	DocsRemovedFromIndex *int `gorm:"default:0"`

	// Nil = no cheap estimate available; UI shows indeterminate progress.
	DocsEstimated *int `gorm:"type:integer"`

	// ErrorMsg / FullExceptionTrace — only populated when Status=failed.
	ErrorMsg           *string `gorm:"type:text"`
	FullExceptionTrace *string `gorm:"type:text"`

	// PollRangeStart / PollRangeEnd — for polling connectors, the
	// window this attempt is fetching.
	PollRangeStart *time.Time
	PollRangeEnd   *time.Time

	// CheckpointPointer — key into the RAG filestore where the
	// in-progress checkpoint blob lives; lets us resume a crashed
	// docfetching run.
	CheckpointPointer *string `gorm:"type:text"`

	// Coordination fields, backed by plain Postgres rows rather than
	// an external fencing mechanism.
	CancellationRequested bool `gorm:"not null;default:false"`

	// Batch coordination.
	//
	// TotalBatches is set once docfetching finishes enumerating work.
	// IsCoordinationComplete() below keys off nil vs populated here.
	TotalBatches            *int `gorm:""`
	CompletedBatches        int  `gorm:"not null;default:0"`
	TotalFailuresBatchLevel int  `gorm:"not null;default:0"`
	TotalChunks             int  `gorm:"not null;default:0"`

	// Stall detection / heartbeat. The watchdog scans
	// `status='in_progress' AND last_progress_time < NOW() -
	// interval`, which is why the partial heartbeat index in
	// indexes.go covers those two columns.
	LastProgressTime          *time.Time
	LastBatchesCompletedCount int `gorm:"not null;default:0"`
	HeartbeatCounter          int `gorm:"not null;default:0"`
	LastHeartbeatValue        int `gorm:"not null;default:0"`
	LastHeartbeatTime         *time.Time

	// Timestamps.
	TimeCreated time.Time `gorm:"not null;default:CURRENT_TIMESTAMP;index"`
	TimeStarted *time.Time
	TimeUpdated time.Time `gorm:"not null;default:CURRENT_TIMESTAMP;autoUpdateTime"`
}

// TableName pins the Postgres table name. All RAG tables use the
// rag_ prefix for namespace isolation from the core Hivy schema.
func (RAGIndexAttempt) TableName() string { return "rag_index_attempts" }

// IsFinished reports whether the attempt has reached a terminal
// state. Proxies to IndexingStatus's IsTerminal so the two cannot
// drift out of sync.
func (a *RAGIndexAttempt) IsFinished() bool {
	return a.Status.IsTerminal()
}

// IsCoordinationComplete returns true once every batch the
// docfetcher enumerated has been fully processed downstream.
//
// Subtle but load-bearing: if TotalBatches is nil the answer is
// always false — docfetching hasn't finished enumerating work yet, so
// we can't possibly have processed "all" of it. CompletedBatches can
// legally exceed TotalBatches on rare races (docfetcher finalised the
// count after a processor already incremented), which is why the
// comparison is >= and not ==.
func (a *RAGIndexAttempt) IsCoordinationComplete() bool {
	if a.TotalBatches == nil {
		return false
	}
	return a.CompletedBatches >= *a.TotalBatches
}
