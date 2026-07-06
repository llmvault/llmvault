package model

import (
	"time"

	"github.com/google/uuid"
)

// RAGSyncRecord is an audit/status row for a "sync" operation — a
// batch job that reconciles a chunk of Postgres state into the vector
// store (deletion-sweep, pruning sweep, external-permissions refresh,
// external-group refresh).
//
// Notes:
//   - We exclude the `document_set` and `user_group` SyncType values
//     because Hivy has no DocumentSet and no UserGroup concept.
//     See SyncType in enums_index_attempt.go.
//   - OrgID column enables row-level tenancy.
//   - PK is a uuid, not an autoincrement int (Hivy convention).
type RAGSyncRecord struct {
	// ID is the primary key.
	ID uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`

	// OrgID — every sync runs inside a single org; the scheduler fans
	// out per-org.
	OrgID uuid.UUID `gorm:"type:uuid;not null;index;constraint:OnDelete:CASCADE"`

	// EntityID — the subject of the sync, stored as a uuid because
	// every Hivy entity is uuid-keyed. Interpretation is driven by
	// SyncType: for `connector_deletion` / `pruning` /
	// `external_permissions` / `external_group` the EntityID refers to
	// an Connection.
	EntityID uuid.UUID `gorm:"type:uuid;not null"`

	// SyncType. See SyncType in enums_index_attempt.go for the Hivy
	// subset.
	SyncType SyncType `gorm:"type:text;not null"`

	// SyncStatus is the lifecycle state of this sync run.
	SyncStatus SyncStatus `gorm:"type:text;not null"`

	// NumDocsSynced — count of docs touched by this sync.
	NumDocsSynced int `gorm:"not null;default:0"`

	// SyncStartTime / SyncEndTime — wall-clock window. End is nullable
	// for still-running syncs.
	SyncStartTime time.Time `gorm:"not null"`
	SyncEndTime   *time.Time
}

// TableName pins the Postgres table name.
func (RAGSyncRecord) TableName() string { return "rag_sync_records" }
