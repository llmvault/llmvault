package model

import (
	"github.com/google/uuid"
)

// RAGUserExternalUserGroup maps a Hivy User (internal or
// external-identified) to the external groups they belong to, scoped by
// the RAGSource that discovered the membership.
//
// Every RAG table uniformly keys off the top-level RAGSource via
// `rag_source_id`.
//
// Stale-sweep semantics (security-critical):
//  1. Sync start: UPDATE ... SET stale = true WHERE rag_source_id = X
//  2. Sync body: upsert fresh rows with stale = false
//  3. Sync end:   DELETE WHERE rag_source_id = X AND stale = true
//
// If the sweep is wrong (e.g. stale rows survive), users retain
// permissions for groups they were removed from upstream. The
// `TestRAGUserExternalUserGroup_StaleSweepPattern` test pins this.
type RAGUserExternalUserGroup struct {
	UserID              uuid.UUID `gorm:"type:uuid;primaryKey"`
	ExternalUserGroupID string    `gorm:"type:text;primaryKey"`
	// RAGSourceID — composite-PK column, FK to rag_sources(id).
	RAGSourceID uuid.UUID `gorm:"type:uuid;primaryKey"`

	// Stale flag for the sync pattern above.
	Stale bool `gorm:"not null;default:false"`
}

func (RAGUserExternalUserGroup) TableName() string { return "rag_user_external_user_groups" }
