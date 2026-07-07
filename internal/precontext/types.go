package precontext

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/model"
)

const (
	TotalBudgetBytes     = 68 * 1024
	SessionsBudgetBytes  = 1200
	KnowledgeBudgetBytes = 800
	MemoriesBudgetBytes  = 64 * 1024
	DefaultCacheTTL      = 24 * time.Hour
)

type Request struct {
	OrgID            uuid.UUID
	AgentID          uuid.UUID
	ChannelID        uuid.UUID
	CurrentSessionID uuid.UUID
	Text             string
	UserID           string
	UserDisplayName  string
	Source           string
	// IncludeOrgMemories folds org-wide memories into this channel's recall
	// (the channel's expose_org_memories flag).
	IncludeOrgMemories bool
}

type Builder interface {
	Build(context.Context, Request) ([]string, error)
}

// MemoryLister is the recall source for the memories section. Recall reads, in
// order of preference: directives (hard rules, always all of them), the
// channel's precomputed memory digest, and top observations (digest missing).
// A channel with none of these gets no memories section. Every method must
// stay cheap and indexed — this runs on the synchronous session-create path.
type MemoryLister interface {
	ActiveDirectives(context.Context, uuid.UUID, memory.ChannelScope) ([]model.AgentDirective, error)
	ChannelMemoryDigest(ctx context.Context, orgID, channelID uuid.UUID) (string, error)
	TopObservations(ctx context.Context, orgID uuid.UUID, scope memory.ChannelScope, limit int) ([]model.AgentObservation, error)
}

// EnvVarDoc is the awareness-only projection of a channel environment
// variable: name and description, never the value. The value is structurally
// unrepresentable here, so it can never reach the rendered precontext even
// through a later bug in this package.
type EnvVarDoc struct {
	Name        string
	Description string
}

// EnvVarLister loads the docs (names + descriptions, never values) of the
// environment variables configured for a channel. Env vars are strictly
// channel-scoped — there are no org-level vars — orgID only narrows the query
// for tenancy. Implementations must never select or decrypt the value column.
type EnvVarLister interface {
	ChannelEnvVars(ctx context.Context, orgID, channelID uuid.UUID) ([]EnvVarDoc, error)
}

type Cache interface {
	Get(context.Context, string) (string, bool, error)
	Set(context.Context, string, string, time.Duration) error
	Del(context.Context, ...string) error
	DeletePrefix(context.Context, string) error
}

type SourceFetcher func(context.Context, Request) (string, error)
