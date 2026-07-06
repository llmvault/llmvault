package model

// Enums for the index-attempt + sync-record tables
// (RAGIndexAttempt, RAGIndexAttemptError, RAGSyncRecord).

// IndexingStatus is the lifecycle status of a single RAGIndexAttempt.
// Value strings are stable on-disk identifiers — they must not change
// once rows exist.
type IndexingStatus string

const (
	IndexingStatusNotStarted          IndexingStatus = "not_started"
	IndexingStatusInProgress          IndexingStatus = "in_progress"
	IndexingStatusSuccess             IndexingStatus = "success"
	IndexingStatusCanceled            IndexingStatus = "canceled"
	IndexingStatusFailed              IndexingStatus = "failed"
	IndexingStatusCompletedWithErrors IndexingStatus = "completed_with_errors"
)

// IsTerminal returns true for every status that the scheduler should
// treat as "finished" (successful or not).
//
// The scheduler uses this to decide whether it's safe to spawn a new
// attempt for the same (connection, embedding model) pair — a wrong
// answer here stalls the indexing queue, so the branch coverage on
// this function is load-bearing.
func (s IndexingStatus) IsTerminal() bool {
	switch s {
	case IndexingStatusSuccess,
		IndexingStatusCompletedWithErrors,
		IndexingStatusCanceled,
		IndexingStatusFailed:
		return true
	default:
		return false
	}
}

// IsSuccessful returns true when the attempt produced usable index
// output. "completed_with_errors" counts as successful — partial
// failures still yield retrievable documents.
func (s IndexingStatus) IsSuccessful() bool {
	return s == IndexingStatusSuccess || s == IndexingStatusCompletedWithErrors
}

// IndexingMode selects full reindex vs incremental update.
type IndexingMode string

const (
	IndexingModeUpdate  IndexingMode = "update"
	IndexingModeReindex IndexingMode = "reindex"
)

// SyncType is the kind of sync operation represented by a
// RAGSyncRecord.
//
// Note: there is intentionally no `document_set` or `user_group`
// value — Hivy has neither concept.
type SyncType string

const (
	SyncTypeConnectorDeletion   SyncType = "connector_deletion"
	SyncTypePruning             SyncType = "pruning"
	SyncTypeExternalPermissions SyncType = "external_permissions"
	SyncTypeExternalGroup       SyncType = "external_group"
)

// IsValid returns true when s is one of the recognised SyncType values.
// Used by API input validation at the edges of the RAG sync coordinator.
func (s SyncType) IsValid() bool {
	switch s {
	case SyncTypeConnectorDeletion,
		SyncTypePruning,
		SyncTypeExternalPermissions,
		SyncTypeExternalGroup:
		return true
	default:
		return false
	}
}

// SyncStatus is the lifecycle state of a RAGSyncRecord.
type SyncStatus string

const (
	SyncStatusInProgress SyncStatus = "in_progress"
	SyncStatusSuccess    SyncStatus = "success"
	SyncStatusFailed     SyncStatus = "failed"
	SyncStatusCanceled   SyncStatus = "canceled"
)

// IsTerminal returns true for sync statuses that conclude the sync
// run. Note the subtle difference vs IndexingStatus.IsTerminal:
// `canceled` is NOT terminal for a sync — it distinguishes "user
// cancelled, still cleaning up" from "truly done". Do not "fix" this
// to match IndexingStatus — it's intentional.
func (s SyncStatus) IsTerminal() bool {
	switch s {
	case SyncStatusSuccess, SyncStatusFailed:
		return true
	default:
		return false
	}
}
