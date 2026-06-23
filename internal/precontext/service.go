package precontext

import (
	"context"
	"fmt"
	"reflect"
	"time"

	"github.com/sourcegraph/conc/pool"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/rag/embedclient"
	"github.com/usehivy/hivy/internal/rag/qdrant"
)

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
	Searcher   KnowledgeSearcher
	Embedder   Embedder
	Reranker   Reranker
	Memories   MemorySearcher
	Collection string
	CacheTTL   time.Duration
}

type Service struct {
	cfg       Config
	sessions  SourceFetcher
	knowledge SourceFetcher
	memories  SourceFetcher
}

func NewService(cfg Config) *Service {
	if cfg.CacheTTL <= 0 {
		cfg.CacheTTL = DefaultCacheTTL
	}
	s := &Service{cfg: cfg}
	s.sessions = s.fetchSessionsSection
	s.knowledge = s.fetchKnowledgeSection
	s.memories = s.fetchMemoriesSection
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
	run := func(index int, name string, fetch SourceFetcher) {
		defer func() {
			if recovered := recover(); recovered != nil {
				logging.Capture(ctx, fmt.Errorf("agent precontext %s panic: %v", name, recovered))
			}
		}()
		text, err := fetch(ctx, req)
		if err != nil {
			logging.Capture(ctx, fmt.Errorf("agent precontext %s: %w", name, err))
			return
		}
		results <- result{index: index, text: text}
	}
	fetchers := pool.New().WithMaxGoroutines(3)
	fetchers.Go(func() { run(0, "sessions", s.cached(SessionsCacheKey(req.OrgID, req.AgentID), s.sessions)) })
	fetchers.Go(func() {
		run(1, "knowledge", s.cached(KnowledgeCacheKey(req.OrgID, req.AgentID, req.Text), s.knowledge))
	})
	fetchers.Go(func() { run(2, "memories", s.memories) })
	fetchers.Wait()
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
				logging.Capture(ctx, fmt.Errorf("agent precontext cache get %s: %w", key, err))
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
				logging.Capture(ctx, fmt.Errorf("agent precontext cache set %s: %w", key, err))
			}
		}
		return value, nil
	}
}

func isNilValue(value any) bool {
	if value == nil {
		return true
	}
	v := reflect.ValueOf(value)
	switch v.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Ptr, reflect.Slice:
		return v.IsNil()
	default:
		return false
	}
}
