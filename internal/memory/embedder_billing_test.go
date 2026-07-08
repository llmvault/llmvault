package memory

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type fakeTokenEmbedder struct {
	dim    uint32
	tokens int
}

func (f fakeTokenEmbedder) Embed(_ context.Context, inputs []string) ([][]float32, int, error) {
	out := make([][]float32, len(inputs))
	for i := range inputs {
		vector := make([]float32, f.dim)
		vector[0] = 1
		out[i] = vector
	}
	return out, f.tokens, nil
}

func TestMemoryEmbedEmitsBillableRowAtQwenPrice(t *testing.T) {
	db := connectMemoryToolTestDB(t)

	org := model.Org{ID: uuid.New(), Name: "memory-embed-bill-" + uuid.NewString(), Active: true, RateLimit: 1000}
	if err := db.Create(&org).Error; err != nil {
		t.Fatalf("create org: %v", err)
	}
	t.Cleanup(func() {
		db.Unscoped().Where("org_id = ?", org.ID).Delete(&model.Generation{})
		db.Unscoped().Delete(&org)
	})

	svc := NewService(Config{
		DB:             db,
		Embedder:       fakeTokenEmbedder{dim: DefaultEmbeddingDim, tokens: 1_000_000},
		EmbeddingModel: DefaultEmbeddingModel,
		EmbeddingDim:   DefaultEmbeddingDim,
	})

	if _, err := svc.EmbedContents(context.Background(), org.ID, []string{"remember this"}); err != nil {
		t.Fatalf("embed contents: %v", err)
	}

	var gen model.Generation
	if err := db.Where("org_id = ?", org.ID).First(&gen).Error; err != nil {
		t.Fatalf("load generation: %v", err)
	}
	if !gen.IsSystem {
		t.Error("memory embedding generation must be is_system to be billable")
	}
	if gen.Model != DefaultEmbeddingModel {
		t.Errorf("model = %q, want %q", gen.Model, DefaultEmbeddingModel)
	}
	// qwen/qwen3-embedding-8b = $0.01 / 1M tokens.
	if gen.Cost != 0.01 {
		t.Errorf("cost = %v, want 0.01 (1M tokens @ $0.01/1M)", gen.Cost)
	}
}
