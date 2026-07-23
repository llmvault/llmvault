package memory

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"
)

type emptyBodyEmbedder struct{}

func (emptyBodyEmbedder) Embed(_ context.Context, _ []string) ([][]float32, int, error) {
	return nil, 0, fmt.Errorf("embed: empty response body (status=%d)", http.StatusOK)
}

// TestEmbedContentsWrapsEmptyBodyError covers the boundary between the
// embedder and the memory service. Issue #214 asks for evidence that the
// empty-body error surfaces clearly through EmbedContents; this test
// asserts the message is informative so the consolidation handler can
// wrap it for asynq retry.
func TestEmbedContentsWrapsEmptyBodyError(t *testing.T) {
	svc := NewService(Config{
		Embedder:       emptyBodyEmbedder{},
		EmbeddingModel: DefaultEmbeddingModel,
		EmbeddingDim:   DefaultEmbeddingDim,
	})

	_, err := svc.EmbedContents(context.Background(), uuid.New(), []string{"hello"})
	if err == nil {
		t.Fatal("expected error from empty-body embedder, got nil")
	}
	if !strings.Contains(err.Error(), "embed: empty response body") {
		t.Fatalf("expected error to surface embedder message, got: %v", err)
	}
	if !strings.Contains(err.Error(), "status=200") {
		t.Fatalf("expected error to include status=200, got: %v", err)
	}
}
