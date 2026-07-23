package memory

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/cache"
	"github.com/usehivy/hivy/internal/model"
	"github.com/usehivy/hivy/internal/rag/embedclient"
)

func (s *Service) EmbedMemoryContent(ctx context.Context, orgID uuid.UUID, content string) ([]float32, error) {
	return s.embedOne(ctx, orgID, content)
}

func (s *Service) EmbedQuery(ctx context.Context, orgID uuid.UUID, query string) ([]float32, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("query is required")
	}
	return s.embedOne(ctx, orgID, "Instruct: Retrieve relevant organization and user memories that help an AI agent answer or act on the user message.\nQuery: "+query)
}

func (s *Service) embedOne(ctx context.Context, orgID uuid.UUID, input string) ([]float32, error) {
	emb, err := s.embedder(ctx)
	if err != nil {
		return nil, err
	}
	vectors, tokens, err := emb.Embed(ctx, []string{input})
	if err != nil {
		return nil, err
	}
	if len(vectors) != 1 {
		return nil, fmt.Errorf("embed returned %d vectors", len(vectors))
	}
	if err := validateVector(vectors[0], s.embeddingDim()); err != nil {
		return nil, err
	}
	s.meterEmbedding(ctx, orgID, tokens)
	return vectors[0], nil
}

func (s *Service) meterEmbedding(ctx context.Context, orgID uuid.UUID, tokens int) {
	billing.RecordEmbeddingUsage(ctx, s.cfg.DB, billing.EmbeddingUsage{
		OrgID:       orgID,
		Model:       s.embeddingModel(),
		TotalTokens: tokens,
		Operation:   billing.EmbeddingOperation,
		RequestPath: "memory.embed",
	})
}

func (s *Service) embedder(ctx context.Context) (Embedder, error) {
	if s.cfg.Embedder != nil {
		return s.cfg.Embedder, nil
	}
	if s.cfg.DB == nil || s.cfg.CacheManager == nil {
		return nil, fmt.Errorf("memory embedding dependencies are not configured")
	}
	cred, err := loadEmbeddingCredential(ctx, s.cfg.DB, s.cfg.CacheManager)
	if err != nil {
		return nil, err
	}
	modelID := strings.TrimPrefix(s.embeddingModel(), EmbeddingProviderID+"/")
	return embedclient.NewEmbedder(embedclient.EmbedderConfig{
		BaseURL: cred.BaseURL,
		APIKey:  string(cred.APIKey),
		Model:   modelID,
		Dim:     s.embeddingDim(),
	}), nil
}

func loadEmbeddingCredential(ctx context.Context, db *gorm.DB, cacheManager *cache.Manager) (*cache.DecryptedCredential, error) {
	var cred model.Credential
	if err := db.WithContext(ctx).
		Where("org_id IS NULL AND revoked_at IS NULL AND provider_id = ?", EmbeddingProviderID).
		Order("created_at ASC").
		First(&cred).Error; err != nil {
		return nil, fmt.Errorf("load system embedding credential: %w", err)
	}
	decrypted, err := cacheManager.GetDecryptedCredentialByID(ctx, cred.ID.String())
	if err != nil {
		return nil, fmt.Errorf("decrypt system embedding credential: %w", err)
	}
	return decrypted, nil
}
