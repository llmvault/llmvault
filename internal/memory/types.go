package memory

import (
	"context"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type Embedder interface {
	Embed(context.Context, []string) ([][]float32, int, error)
}

type EnqueueEmbeddingFunc func(context.Context, uuid.UUID, int) error

// AgentScope selects records owned by one agent. Memory is intentionally never
// inherited from the team or another agent: shared team knowledge lives in
// team-owned sources, while retained context is agent-specific.
type AgentScope struct {
	AgentID   uuid.UUID
	AllAgents bool // administration-only listing within an org
}

// whereSQL renders the scope as a SQL predicate and its args. An empty string
// means "no agent filter" (AllAgents).
func (as AgentScope) whereSQL() (string, []any) {
	switch {
	case as.AllAgents:
		return "", nil
	default:
		return "agent_id = ?", []any{as.AgentID}
	}
}

type CreateRequest struct {
	OrgID             uuid.UUID
	AgentID           uuid.UUID
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

// Visibility is retained as an API compatibility placeholder while the HTTP
// layer moves to agent/team authorization. Memory service reads never perform
// legacy visibility checks.
type Visibility struct {
	Restrict bool
	UserID   *uuid.UUID
}

type ListRequest struct {
	OrgID      uuid.UUID
	Scope      AgentScope
	Visibility Visibility
	Tags       []string
	Limit      int
	NoLimit    bool
}

type SearchRequest struct {
	OrgID       uuid.UUID
	Scope       AgentScope
	Visibility  Visibility
	Query       string
	QueryVector []float32
	Tags        []string
	Limit       int
}

type SearchHit struct {
	Memory     model.AgentMemory
	Similarity float64
}
