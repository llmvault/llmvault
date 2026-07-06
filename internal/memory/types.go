package memory

import (
	"context"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type Embedder interface {
	Embed(context.Context, []string) ([][]float32, error)
}

type EnqueueEmbeddingFunc func(context.Context, uuid.UUID, int) error

// ChannelScope selects which memories a read touches. It is the single scoping
// primitive: a regular agent reads its own channel (optionally folding in
// org-wide memories when the channel allows it); the manage tool reads every
// channel.
type ChannelScope struct {
	ChannelID          *uuid.UUID // channel to read; nil = org-wide (channel_id IS NULL) only
	IncludeOrgMemories bool       // also include org-wide memories (only meaningful with ChannelID set)
	AllChannels        bool       // manage mode: every memory in the org, any channel
}

// whereSQL renders the scope as a SQL predicate and its args. An empty string
// means "no channel filter" (AllChannels).
func (cs ChannelScope) whereSQL() (string, []any) {
	switch {
	case cs.AllChannels:
		return "", nil
	case cs.ChannelID != nil && cs.IncludeOrgMemories:
		return "(channel_id = ? OR channel_id IS NULL)", []any{*cs.ChannelID}
	case cs.ChannelID != nil:
		return "channel_id = ?", []any{*cs.ChannelID}
	default:
		return "channel_id IS NULL", nil
	}
}

type CreateRequest struct {
	OrgID             uuid.UUID
	ChannelID         *uuid.UUID // nil = org-wide
	Content           string
	MemoryFingerprint string
	Tags              []string
	Metadata          model.JSON
	SourceSessionID   *uuid.UUID
	SourceEventID     *uuid.UUID
	CreatedByUserID   *uuid.UUID
}

type UpdateRequest struct {
	OrgID    uuid.UUID
	ID       uuid.UUID
	Content  *string
	Tags     *[]string
	Metadata *model.JSON
}

type ArchiveRequest struct {
	OrgID uuid.UUID
	ID    uuid.UUID
}

type ListRequest struct {
	OrgID   uuid.UUID
	Scope   ChannelScope
	Tags    []string
	Limit   int
	NoLimit bool
}

type SearchRequest struct {
	OrgID       uuid.UUID
	Scope       ChannelScope
	Query       string
	QueryVector []float32
	Tags        []string
	Limit       int
}

type SearchHit struct {
	Memory     model.AgentMemory
	Similarity float64
}
