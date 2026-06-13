package precontext

import (
	"context"
	"time"

	"github.com/google/uuid"
)

const (
	TotalBudgetBytes     = 3 * 1024
	SessionsBudgetBytes  = 1200
	MemoriesBudgetBytes  = 1000
	KnowledgeBudgetBytes = 800
	DefaultCacheTTL      = 24 * time.Hour
)

type Request struct {
	OrgID                 uuid.UUID
	AgentID               uuid.UUID
	CurrentSessionID      uuid.UUID
	Text                  string
	UserID                string
	UserDisplayName       string
	Source                string
}

type Builder interface {
	Build(context.Context, Request) ([]string, error)
}

type Cache interface {
	Get(context.Context, string) (string, bool, error)
	Set(context.Context, string, string, time.Duration) error
	Del(context.Context, ...string) error
	DeletePrefix(context.Context, string) error
}

type SourceFetcher func(context.Context, Request) (string, error)
