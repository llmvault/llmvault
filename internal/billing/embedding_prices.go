package billing

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/lib/pq"
	"github.com/oklog/ulid/v2"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/usehivy/hivy/internal/logging"
	"github.com/usehivy/hivy/internal/model"
)

const EmbeddingOperation = "embedding"

var embeddingPricesPer1M = map[string]float64{
	"openai/text-embedding-3-small": 0.02,
	"qwen/qwen3-embedding-8b":       0.01,
}

func EmbeddingPricePer1M(modelSlug string) (float64, bool) {
	price, ok := embeddingPricesPer1M[strings.TrimSpace(modelSlug)]
	return price, ok
}

func EmbeddingCostUSD(modelSlug string, totalTokens int) float64 {
	if totalTokens <= 0 {
		return 0
	}
	price, ok := EmbeddingPricePer1M(modelSlug)
	if !ok {
		return 0
	}
	return float64(totalTokens) * price / 1_000_000
}

type EmbeddingUsage struct {
	OrgID        uuid.UUID
	Model        string
	TotalTokens  int
	CredentialID uuid.UUID
	Operation    string
	RequestPath  string
	UserID       string
}

func RecordEmbeddingUsage(ctx context.Context, db *gorm.DB, usage EmbeddingUsage) {
	if db == nil || usage.OrgID == uuid.Nil || usage.TotalTokens <= 0 {
		return
	}
	slug := strings.TrimSpace(usage.Model)
	if _, ok := EmbeddingPricePer1M(slug); !ok {
		logging.FromContext(ctx).WarnContext(ctx, "embedding model has no configured price",
			"model", slug, "org_id", usage.OrgID.String(), "total_tokens", usage.TotalTokens)
	}
	operation := strings.TrimSpace(usage.Operation)
	if operation == "" {
		operation = EmbeddingOperation
	}
	gen := model.Generation{
		ID:             "gen_" + ulid.Make().String(),
		OrgID:          usage.OrgID,
		CredentialID:   usage.CredentialID,
		TokenJTI:       "system:" + operation,
		ProviderID:     embeddingProviderID(slug),
		Model:          slug,
		RequestPath:    strings.TrimSpace(usage.RequestPath),
		InputTokens:    usage.TotalTokens,
		Cost:           EmbeddingCostUSD(slug, usage.TotalTokens),
		UpstreamStatus: 200,
		UserID:         strings.TrimSpace(usage.UserID),
		Tags:           pq.StringArray{"model_usage", operation},
		CreatedAt:      time.Now().UTC(),
		IsSystem:       true,
	}
	if err := db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "id"}},
		DoNothing: true,
	}).Create(&gen).Error; err != nil {
		logging.Capture(ctx, err)
	}
}

func embeddingProviderID(slug string) string {
	if idx := strings.IndexByte(slug, '/'); idx > 0 {
		return slug[:idx]
	}
	return ""
}
