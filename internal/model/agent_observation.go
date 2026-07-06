package model

import (
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
)

// AgentObservation is the consolidated memory product built from raw
// reflection facts (agent_memories): one canonical observation per facet,
// carrying proof counts, supersession links, expiry, and channel scoping.
// ChannelID scopes the observation: set = that channel's memory, NULL =
// org-wide. Facts stay as internal evidence behind SourceFactIDs.
type AgentObservation struct {
	ID        uuid.UUID      `gorm:"type:uuid;primaryKey;default:gen_random_uuid()"`
	OrgID     uuid.UUID      `gorm:"type:uuid;not null;index"`
	Org       Org            `gorm:"foreignKey:OrgID;constraint:OnDelete:CASCADE"`
	ChannelID *uuid.UUID     `gorm:"type:uuid;index"`
	Channel   *Channel       `gorm:"foreignKey:ChannelID;constraint:OnDelete:CASCADE"`
	Content   string         `gorm:"type:text;not null"`
	Kind      string         `gorm:"type:text;not null"`
	Entities  pq.StringArray `gorm:"type:text[];not null;default:'{}'"`
	// ProofCount is how many source facts support this observation.
	ProofCount int `gorm:"not null;default:1"`
	// SourceFactIDs reference agent_memories.id rows (stored as uuid[]).
	SourceFactIDs   pq.StringArray `gorm:"type:uuid[];not null;default:'{}';column:source_fact_ids"`
	OccurredStart   *time.Time
	OccurredEnd     *time.Time
	LastMentionedAt time.Time `gorm:"not null"`
	ExpiresAt       *time.Time
	SupersededBy    *uuid.UUID `gorm:"type:uuid"`
	ArchivedAt      *time.Time
	// HumanVerified marks user-confirmed content: consolidation may append
	// evidence but never rewrites the content text.
	HumanVerified     bool   `gorm:"not null;default:false"`
	EmbeddingModel    string `gorm:"type:text;not null;default:''"`
	EmbeddingStatus   string `gorm:"type:text;not null;default:'pending'"`
	EmbeddingRevision int    `gorm:"not null;default:1"`
	EmbeddingError    string `gorm:"type:text;not null;default:''"`
	EmbeddedAt        *time.Time
	// Metadata carries the consolidation audit trail under "audit": an array
	// of {op, reason, fact_ids, at} entries appended on every applied op.
	Metadata  JSON `gorm:"type:jsonb;not null;default:'{}'"`
	CreatedAt time.Time
	UpdatedAt time.Time
}

func (AgentObservation) TableName() string { return "agent_observations" }

// ChannelMemoryDigest is the precomputed, byte-budgeted memory block for one
// channel: ranked and rendered at write time by the consolidation worker so
// session create is a single point-read. Org-wide observations are folded
// into each channel digest at write time — there is no separate org row.
type ChannelMemoryDigest struct {
	ChannelID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	OrgID            uuid.UUID `gorm:"type:uuid;not null"`
	Content          string    `gorm:"type:text;not null"`
	ObservationCount int       `gorm:"not null;default:0"`
	UpdatedAt        time.Time
}

func (ChannelMemoryDigest) TableName() string { return "channel_memory_digests" }
