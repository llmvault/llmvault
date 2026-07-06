package model

import (
	"time"

	"github.com/google/uuid"
)

// RAGSearchSettings holds the per-org embedding + search configuration.
//
// Notes:
//
//  1. Per-org, not global. OrgID is the primary key; one settings row
//     per org. Hivy's "one model per org for the lifetime of their
//     index" invariant means there's no need for multiple coexisting
//     settings rows or a model-switchover status machine. If an org
//     wants a new embedding model, ops deletes their chunks and
//     re-ingests; the settings row is mutated in place.
//
//  2. FK EmbeddingModelID → rag_embedding_models(id). The catalog
//     table is created by embedder.Migrate before rag model.Migrate
//     installs this FK (see RAG goose migrations).
//
//  3. RerankerModelID (per-org reranker choice — Qwen3-Reranker-0.6B
//     default), HybridAlpha (BM25/vector weight for hybrid search,
//     default 0.7), and the three ContextualRAG fields are reserved
//     for a future contextual-RAG feature.
type RAGSearchSettings struct {
	// OrgID is the primary key — one settings row per org. FK CASCADE
	// so org deletion wipes the settings row along with everything
	// else.
	OrgID uuid.UUID `gorm:"type:uuid;primaryKey"`

	// EmbeddingModelID references rag_embedding_models.id. The FK
	// constraint is installed by model.Migrate after embedder.Migrate
	// has created the catalog table.
	EmbeddingModelID string `gorm:"type:varchar(128);not null;index"`

	// EmbeddingDim is denormalized from the model catalog so downstream
	// queries don't need a join to know the vector width.
	EmbeddingDim int `gorm:"not null"`

	// Normalize — whether to L2-normalize embedding outputs before
	// storage.
	Normalize bool `gorm:"not null;default:true"`

	// QueryPrefix / PassagePrefix — some models (e.g. E5,
	// Qwen3-Embedding) expect a text prefix to disambiguate
	// query-vs-document embeddings.
	QueryPrefix   *string `gorm:"type:text"`
	PassagePrefix *string `gorm:"type:text"`

	// EmbeddingPrecision selects the numeric precision used when
	// writing vectors.
	EmbeddingPrecision EmbeddingPrecision `gorm:"type:varchar(16);not null;default:'float'"`

	// ReducedDimension — optional Matryoshka-style truncation (OpenAI
	// models support this).
	ReducedDimension *int `gorm:"type:integer"`

	// MultipassIndexing enables the "mini + large chunks" strategy.
	MultipassIndexing bool `gorm:"not null;default:true"`

	// RerankerModelID points at a reranker registered in the same
	// rag_embedding_models catalog (or a separate reranker catalog in
	// future). Nullable = no reranking.
	RerankerModelID *string `gorm:"type:varchar(128)"`

	// HybridAlpha — weight on the vector score in hybrid search (0 =
	// pure BM25, 1 = pure vector). Default 0.7 per plan's "locked
	// stack decisions" table.
	HybridAlpha float64 `gorm:"type:double precision;not null;default:0.7"`

	// IndexName names the underlying vector-store index (Qdrant
	// collection/index name). For Hivy this is derived from
	// `rag_embedding_models.DatasetName` at ingest time; persisted
	// here so ops can pin an org to a specific index during ops work.
	IndexName string `gorm:"type:varchar(256);not null"`

	// EnableContextualRAG. Reserved for a future contextual-RAG feature.
	EnableContextualRAG bool `gorm:"not null;default:false"`

	// ContextualRAGLLMName / ContextualRAGLLMProvider. Reserved for a
	// future contextual-RAG feature.
	ContextualRAGLLMName     *string `gorm:"type:varchar(128)"`
	ContextualRAGLLMProvider *string `gorm:"type:varchar(64)"`

	CreatedAt time.Time
	UpdatedAt time.Time
}

// TableName — Hivy `rag_*` convention.
func (RAGSearchSettings) TableName() string { return "rag_search_settings" }
