package model

import (
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

// RAGSyncState tracks the connection lifecycle and sync bookkeeping for
// a source.
//
// ARCHITECTURAL NOTE: identity lives on `Connection`, schedule +
// config live on `RAGSource`, and sync state (this struct) is a 1:1
// sibling of `RAGSource` keyed by `rag_source_id`. The uniqueness of
// `rag_source_id` is the invariant the three-loop scheduler (docfetching,
// docprocessing, pruning, doc-permission syncing, external-group
// syncing) depends on: every loop picks up at most one sync state per
// source.
//
// Identity columns (`connector_id`, `credential_id`, `name`) live on
// Connection, not here.
type RAGSyncState struct {
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	// OrgID carries row-level tenancy — every RAG row carries org_id
	// with CASCADE.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index"`

	// RAGSourceID — 1:1 link to RAGSource. Unique: a source has at most
	// one sync state. CASCADE means deleting the source zaps its sync
	// metadata. INTEGRATION-kind sources carry the Connection reference
	// on the RAGSource row itself; WEBSITE / FILE_UPLOAD sources don't
	// have one.
	RAGSourceID uuid.UUID `gorm:"type:uuid;not null;uniqueIndex:uq_rag_sync_state_rag_source_id"`

	// Status drives the scheduler's "should I run?" gate; see
	// RAGConnectionStatus.IsActive.
	Status RAGConnectionStatus `gorm:"type:varchar(32);not null"`

	// InRepeatedErrorState is orthogonal to Status; a connection can be
	// ACTIVE but flagged as repeatedly failing, which the UI surfaces
	// without pausing the loop.
	InRepeatedErrorState bool `gorm:"not null;default:false"`

	// AccessType controls who can see documents from this source.
	AccessType AccessType `gorm:"type:varchar(16);not null"`

	// AutoSyncOptions (JSONB) — shape is connector-specific (e.g.
	// `{"customer_id": "...", "company_domain": "..."}` for Google
	// Drive perm sync).
	AutoSyncOptions model.JSON `gorm:"type:jsonb"`

	// LastTimePermSync records when permissions were last synced.
	LastTimePermSync *time.Time `gorm:"type:timestamptz"`

	// LastTimeExternalGroupSync records when external groups were last
	// synced.
	LastTimeExternalGroupSync *time.Time `gorm:"type:timestamptz"`

	// LastSuccessfulIndexTime is the finish time (not start time) of
	// the last successful index run.
	LastSuccessfulIndexTime *time.Time `gorm:"type:timestamptz"`

	// LastPruned records when the pruning sweep last ran.
	LastPruned *time.Time `gorm:"type:timestamptz;index:idx_rag_sync_state_last_pruned"`

	// LastTimeHierarchyFetch records when the source hierarchy was
	// last fetched.
	LastTimeHierarchyFetch *time.Time `gorm:"type:timestamptz"`

	// TotalDocsIndexed is a running count of docs indexed for this
	// source.
	TotalDocsIndexed int `gorm:"not null;default:0"`

	// IndexingTrigger is nullable: a one-shot signal the API sets to
	// ask the scheduler to do `update` vs `reindex` on the next pass.
	// Typed as *string to avoid the redundant import of IndexingMode
	// at this site; the Postgres column accepts the same values
	// either way.
	IndexingTrigger *string `gorm:"type:varchar(16)"`

	// ProcessingMode selects the post-fetch pipeline for this source.
	ProcessingMode ProcessingMode `gorm:"type:varchar(16);not null;default:'REGULAR'"`

	// DeletionFailureMessage records why a deletion attempt failed, if
	// it did.
	DeletionFailureMessage *string `gorm:"type:text"`

	// CreatorID is the user who created the underlying source.
	// Nullable because deletion of the user should not cascade into
	// tombstoning the sync state; the row is owned by the org, not the
	// creator.
	CreatorID *uuid.UUID `gorm:"type:uuid"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName — follow Hivy convention (`orgs`, `connections`,
// etc.); RAG tables prefixed `rag_`.
func (RAGSyncState) TableName() string { return "rag_sync_states" }
