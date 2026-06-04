package precontext

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/hindsight"
	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/rag/embedclient"
	"github.com/usehivy/hivy/internal/rag/qdrant"
)

type MemoryLister interface {
	ListMemories(context.Context, string, int, int) (*hindsight.ListMemoriesResponse, error)
}

type MemoryBankEnsurer interface {
	EnsureOrgBank(context.Context, uuid.UUID) error
}

type Embedder interface {
	Embed(context.Context, []string) ([][]float32, error)
}

type KnowledgeSearcher interface {
	Search(context.Context, qdrant.SearchRequest) ([]qdrant.Hit, error)
}

type Reranker interface {
	Rerank(context.Context, string, []string, int) ([]embedclient.RerankResult, error)
}

type Config struct {
	DB         *gorm.DB
	Cache      Cache
	Memory     MemoryLister
	MemoryBank MemoryBankEnsurer
	Searcher   KnowledgeSearcher
	Embedder   Embedder
	Reranker   Reranker
	Collection string
	CacheTTL   time.Duration
}

type Service struct {
	cfg       Config
	sessions  SourceFetcher
	memories  SourceFetcher
	knowledge SourceFetcher
}

func NewService(cfg Config) *Service {
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = DefaultCacheTTL
	}
	s := &Service{cfg: cfg}
	s.sessions = s.fetchSessionsSection
	s.memories = s.fetchMemoriesSection
	s.knowledge = s.fetchKnowledgeSection
	return s
}

func (s *Service) Build(ctx context.Context, req Request) ([]string, error) {
	if s == nil {
		return nil, nil
	}
	type result struct {
		index int
		text  string
	}
	results := make(chan result, 3)
	var wg sync.WaitGroup
	run := func(index int, name string, fetch SourceFetcher) {
		defer wg.Done()
		text, err := fetch(ctx, req)
		if err != nil {
			logging.Capture(ctx, fmt.Errorf("employee precontext %s: %w", name, err))
			return
		}
		results <- result{index: index, text: text}
	}
	wg.Add(3)
	go run(0, "sessions", s.cached(SessionsCacheKey(req.OrgID, req.EmployeeID), s.sessions))
	go run(1, "memories", s.cached(MemoriesCacheKey(req.OrgID, req.EmployeeID), s.memories))
	go run(2, "knowledge", s.cached(KnowledgeCacheKey(req.OrgID, req.EmployeeID, req.Text), s.knowledge))
	wg.Wait()
	close(results)

	ordered := make([]string, 3)
	for res := range results {
		ordered[res.index] = res.text
	}
	combined := joinSections(ordered, TotalBudgetBytes)
	if combined == "" {
		return nil, nil
	}
	return []string{combined}, nil
}

func (s *Service) cached(key string, fetch SourceFetcher) SourceFetcher {
	return func(ctx context.Context, req Request) (string, error) {
		if s.cfg.Cache != nil {
			if value, hit, err := s.cfg.Cache.Get(ctx, key); err != nil {
				logging.Capture(ctx, fmt.Errorf("employee precontext cache get %s: %w", key, err))
			} else if hit {
				return value, nil
			}
		}
		value, err := fetch(ctx, req)
		if err != nil {
			return "", err
		}
		if s.cfg.Cache != nil {
			if err := s.cfg.Cache.Set(ctx, key, value, s.cfg.CacheTTL); err != nil {
				logging.Capture(ctx, fmt.Errorf("employee precontext cache set %s: %w", key, err))
			}
		}
		return value, nil
	}
}
