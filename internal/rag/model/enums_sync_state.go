package model

// Enums for the sync-state + search-settings tables.

// RAGConnectionStatus is the lifecycle state of a connection's sync
// state.
//
// Values are uppercase names (not lowercase values) to preserve on-disk
// compatibility with existing stored data.
type RAGConnectionStatus string

const (
	RAGConnectionStatusScheduled       RAGConnectionStatus = "SCHEDULED"
	RAGConnectionStatusInitialIndexing RAGConnectionStatus = "INITIAL_INDEXING"
	RAGConnectionStatusActive          RAGConnectionStatus = "ACTIVE"
	RAGConnectionStatusPaused          RAGConnectionStatus = "PAUSED"
	RAGConnectionStatusDeleting        RAGConnectionStatus = "DELETING"
	RAGConnectionStatusInvalid         RAGConnectionStatus = "INVALID"
)

// ActiveStatuses returns the set of statuses considered "active" for
// scheduling purposes. Returned slice is a fresh copy so callers can
// mutate without polluting subsequent calls.
func ActiveStatuses() []RAGConnectionStatus {
	return []RAGConnectionStatus{
		RAGConnectionStatusActive,
		RAGConnectionStatusScheduled,
		RAGConnectionStatusInitialIndexing,
	}
}

// IndexableStatuses is a superset of ActiveStatuses that also includes
// PAUSED, since paused connections still need to be indexable during
// an embedding-model swap.
func IndexableStatuses() []RAGConnectionStatus {
	return append(ActiveStatuses(), RAGConnectionStatusPaused)
}

// IsActive reports whether s is one of the active statuses.
func (s RAGConnectionStatus) IsActive() bool {
	for _, v := range ActiveStatuses() {
		if s == v {
			return true
		}
	}
	return false
}

// AccessType controls who can see documents from a source.
type AccessType string

const (
	AccessTypePublic  AccessType = "public"
	AccessTypePrivate AccessType = "private"
	AccessTypeSync    AccessType = "sync"
)

// ProcessingMode controls the docfetching branch that selects the
// post-fetch pipeline (full chunk -> embed -> store vs FS drop vs raw
// binary drop).
type ProcessingMode string

const (
	ProcessingModeRegular    ProcessingMode = "REGULAR"
	ProcessingModeFileSystem ProcessingMode = "FILE_SYSTEM"
	ProcessingModeRawBinary  ProcessingMode = "RAW_BINARY"
)

// EmbeddingPrecision selects the numeric precision used when writing
// embedding vectors to the vector store.
type EmbeddingPrecision string

const (
	EmbeddingPrecisionBFloat16 EmbeddingPrecision = "bfloat16"
	EmbeddingPrecisionFloat    EmbeddingPrecision = "float"
)
