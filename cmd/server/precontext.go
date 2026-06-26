package main

import (
	"context"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/enqueue"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/precontext"
	"github.com/usehivy/hivy/internal/tasks"
)

func buildMemorySearchService(cfg *config.Config, db *gorm.DB, cacheManager *cache.Manager) *memory.Service {
	return memory.NewService(buildMemoryServiceConfig(cfg, db, cacheManager))
}

func buildMemoryServiceConfig(cfg *config.Config, db *gorm.DB, cacheManager *cache.Manager) memory.Config {
	memCfg := memory.Config{DB: db, CacheManager: cacheManager}
	if cfg != nil {
		memCfg.EmbeddingModel = cfg.MemoryEmbeddingModel
		memCfg.EmbeddingDim = cfg.MemoryEmbeddingDim
	}
	return memCfg
}

func buildMemoryToolService(cfg *config.Config, db *gorm.DB, cacheManager *cache.Manager, enq enqueue.TaskEnqueuer) *memory.Service {
	memCfg := buildMemoryServiceConfig(cfg, db, cacheManager)
	memCfg.EnqueueEmbed = func(ctx context.Context, id uuid.UUID, revision int) error {
		return tasks.EnqueueMemoryEmbed(ctx, enq, id, revision)
	}
	return memory.NewService(memCfg)
}

func buildPreContextService(
	cfg *config.Config,
	db *gorm.DB,
	cache precontext.Cache,
	memories precontext.MemoryLister,
	searcher precontext.KnowledgeSearcher,
	embedder precontext.Embedder,
	reranker precontext.Reranker,
) precontext.Builder {
	pcCfg := precontext.Config{
		DB:       db,
		Cache:    cache,
		Searcher: searcher,
		Embedder: embedder,
		Reranker: reranker,
		Memories: memories,
	}
	if cfg != nil {
		pcCfg.Collection = cfg.QdrantCollection
	}
	return precontext.NewService(pcCfg)
}
