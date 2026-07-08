package model

import (
	"time"

	"github.com/google/uuid"
)

// TeamRagSource provisions a knowledge (RAG) source to a team. The set of rows
// for a team is its knowledge allowlist. Unique per (team, rag_source).
//
// RagSourceID has no GORM association here because RAGSource lives in
// internal/rag/model; the foreign key is enforced by the SQL migration.
type TeamRagSource struct {
	ID            uuid.UUID  `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID         uuid.UUID  `gorm:"type:uuid;not null;index:idx_team_rag_sources_org_team,priority:1"`
	TeamID        uuid.UUID  `gorm:"type:uuid;not null;index:idx_team_rag_sources_org_team,priority:2;uniqueIndex:team_rag_sources_team_source_unique,priority:1"`
	Team          Team       `gorm:"foreignKey:TeamID;constraint:OnDelete:CASCADE"`
	RagSourceID   uuid.UUID  `gorm:"type:uuid;not null;uniqueIndex:team_rag_sources_team_source_unique,priority:2"`
	GrantedBy     *uuid.UUID `gorm:"type:uuid"`
	GrantedByUser *User      `gorm:"foreignKey:GrantedBy;constraint:OnDelete:SET NULL"`
	CreatedAt     time.Time
}

func (TeamRagSource) TableName() string { return "team_rag_sources" }
