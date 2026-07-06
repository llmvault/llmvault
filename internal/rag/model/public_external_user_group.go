package model

import (
	"github.com/google/uuid"
)

// RAGPublicExternalUserGroup stores external "groups" that represent
// anyone-with-the-link / anyone-in-the-domain style public shares. At
// query time the ACL allow-list for a user is extended with every
// (public_external_user_group_id) their sources have recorded, letting
// those public shares show up in search results.
//
// The RAGSourceID keys the row to the top-level RAGSource that
// discovered it. See the stale-sweep doc on `RAGUserExternalUserGroup`
// for the security-critical sync pattern the `stale` flag + indexes
// implement here too.
type RAGPublicExternalUserGroup struct {
	ExternalUserGroupID string `gorm:"type:text;primaryKey"`
	// RAGSourceID — composite-PK column, FK to rag_sources(id).
	RAGSourceID uuid.UUID `gorm:"type:uuid;primaryKey"`

	// Stale flag for the sync pattern.
	Stale bool `gorm:"not null;default:false"`
}

func (RAGPublicExternalUserGroup) TableName() string { return "rag_public_external_user_groups" }
