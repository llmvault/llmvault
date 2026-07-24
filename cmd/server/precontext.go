package main

import (
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/memory"
	"github.com/usehivy/hivy/internal/precontext"
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
	if db != nil {
		pcCfg.EnvVars = precontext.NewGormEnvVarLister(db)
	}
	if cfg != nil {
		pcCfg.Collection = cfg.QdrantCollection
	}
	return precontext.NewService(pcCfg)
}
