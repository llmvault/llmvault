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
	CurrentSessionID uuid.UUID
	Text             string
	UserID           string
	UserDisplayName  string
	Source           string
}

type Builder interface {
	Build(context.Context, Request) ([]string, error)
}

type MemoryLister interface {
	List(context.Context, memory.ListRequest) ([]model.AgentMemory, error)
}

type Cache interface {
	Get(context.Context, string) (string, bool, error)
	Set(context.Context, string, string, time.Duration) error
	Del(context.Context, ...string) error
	DeletePrefix(context.Context, string) error
}

type SourceFetcher func(context.Context, Request) (string, error)
