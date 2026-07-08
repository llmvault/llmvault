package billing_test

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/billing"
	"github.com/usehivy/hivy/internal/model"
)

func TestEmbeddingCostUSD(t *testing.T) {
	cases := []struct {
		name   string
		model  string
		tokens int
		want   float64
	}{
		{"rag_text_embedding_3_small", "openai/text-embedding-3-small", 1_000_000, 0.02},
		{"memory_qwen3_embedding_8b", "qwen/qwen3-embedding-8b", 1_000_000, 0.01},
		{"unknown_model", "acme/unknown", 1_000_000, 0},
		{"zero_tokens", "openai/text-embedding-3-small", 0, 0},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := billing.EmbeddingCostUSD(tc.model, tc.tokens); got != tc.want {
				t.Errorf("EmbeddingCostUSD(%q, %d) = %v, want %v", tc.model, tc.tokens, got, tc.want)
			}
		})
	}
}

func TestIntegration_RecordEmbeddingUsage(t *testing.T) {
	db := connectCreditsTestDB(t)

	cases := []struct {
		name     string
		model    string
		tokens   int
		wantCost float64
	}{
		{"rag_search", "openai/text-embedding-3-small", 500_000, 0.01},
		{"memory_embed", "qwen/qwen3-embedding-8b", 500_000, 0.005},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			orgID := seedOrg(t, db)
			t.Cleanup(func() {
				db.Unscoped().Where("org_id = ?", orgID).Delete(&model.Generation{})
			})

			billing.RecordEmbeddingUsage(context.Background(), db, billing.EmbeddingUsage{
				OrgID:       orgID,
				Model:       tc.model,
				TotalTokens: tc.tokens,
				Operation:   billing.EmbeddingOperation,
			})

			var gen model.Generation
			if err := db.Where("org_id = ?", orgID).First(&gen).Error; err != nil {
				t.Fatalf("load generation: %v", err)
			}
			if !gen.IsSystem {
				t.Error("generation is not is_system; billing batch only bills system rows")
			}
			if gen.Model != tc.model {
				t.Errorf("model = %q, want %q", gen.Model, tc.model)
			}
			if gen.InputTokens != tc.tokens {
				t.Errorf("input_tokens = %d, want %d", gen.InputTokens, tc.tokens)
			}
			if gen.Cost != tc.wantCost {
				t.Errorf("cost = %v, want %v", gen.Cost, tc.wantCost)
			}
			hasEmbeddingTag := false
			for _, tag := range gen.Tags {
				if tag == billing.EmbeddingOperation {
					hasEmbeddingTag = true
				}
			}
			if !hasEmbeddingTag {
				t.Errorf("tags %v missing %q", gen.Tags, billing.EmbeddingOperation)
			}
		})
	}
}
