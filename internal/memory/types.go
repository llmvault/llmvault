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

type CreateRequest struct {
	OrgID             uuid.UUID
	AgentID           *uuid.UUID
	UserID            *uuid.UUID
	Scope             string
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
	AgentID  *uuid.UUID
	Content  *string
	Tags     *[]string
	Metadata *model.JSON
}

type ArchiveRequest struct {
	OrgID uuid.UUID
	ID    uuid.UUID
	// AgentID, when set, restricts archiving to memories visible to that
	// agent (agent_id IS NULL or agent_id = AgentID). Used by the agent
	// tool path; the org-admin REST path leaves it nil.
	AgentID *uuid.UUID
}

type ListRequest struct {
	OrgID           uuid.UUID
	UserID          *uuid.UUID
	AgentID         *uuid.UUID
	Scope           string
	AgentVisibility string
	Tags            []string
	Limit           int
	NoLimit         bool
}

type SearchRequest struct {
	OrgID           uuid.UUID
	UserID          *uuid.UUID
	AgentID         uuid.UUID
	Scope           string
	AgentVisibility string
	Query           string
	QueryVector     []float32
	Tags            []string
	Limit           int
}

type SearchHit struct {
	Memory     model.AgentMemory
	Similarity float64
}

type TurnMemories struct {
	Org  []SearchHit
	User []SearchHit
}
