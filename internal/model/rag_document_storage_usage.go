package model

import (
	"time"

	"github.com/google/uuid"
)

// RAGDocumentStorageUsage records the logical indexed footprint of one source
// document. Re-ingesting the same document replaces this value instead of
// accumulating it, keeping org knowledge quotas stable across refreshes.
type RAGDocumentStorageUsage struct {
	OrgID        uuid.UUID `gorm:"type:uuid;not null;index"`
	RAGSourceID  uuid.UUID `gorm:"type:uuid;not null;primaryKey"`
	DocumentID   string    `gorm:"type:text;not null;primaryKey"`
	StorageBytes int64     `gorm:"not null"`
	CreatedAt    time.Time
	UpdatedAt    time.Time
}

func (RAGDocumentStorageUsage) TableName() string { return "rag_document_storage_usage" }
