package model

import (
	"time"

	"github.com/google/uuid"
)

// ChannelRagSource grants a channel access to one RAG source. The set of rows
// for a channel is its knowledge-base allowlist: search_knowledge_base run from
// a session on that channel returns hits only from the granted sources. A
// channel with no rows has no knowledge access (deny-by-default).
//
// RagSourceID has no GORM association here because RAGSource lives in
// internal/rag/model; the foreign key is enforced by the SQL migration.
type ChannelRagSource struct {
	ID          uuid.UUID `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID       uuid.UUID `gorm:"type:uuid;not null;index"`
	ChannelID   uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_channel_rag_sources_channel_source,priority:1"`
	Channel     Channel   `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE"`
	RagSourceID uuid.UUID `gorm:"type:uuid;not null;index;uniqueIndex:idx_channel_rag_sources_channel_source,priority:2"`
	CreatedAt   time.Time
}

func (ChannelRagSource) TableName() string { return "channel_rag_sources" }
