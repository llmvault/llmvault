package handler

import (
	"testing"

	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

func iptr(v int) *int { return &v }

func TestComputeRAGProgress(t *testing.T) {
	cases := []struct {
		name          string
		attempt       ragmodel.RAGIndexAttempt
		wantPercent   *int
		wantBasis     string
		wantIndet     bool
		wantDocs      int
	}{
		{
			name:        "successful attempt is 100 percent",
			attempt:     ragmodel.RAGIndexAttempt{Status: ragmodel.IndexingStatusSuccess, TotalDocsIndexed: iptr(7)},
			wantPercent: iptr(100),
			wantBasis:   "complete",
			wantDocs:    7,
		},
		{
			name:        "docs_estimated drives percent",
			attempt:     ragmodel.RAGIndexAttempt{Status: ragmodel.IndexingStatusInProgress, TotalDocsIndexed: iptr(25), DocsEstimated: iptr(100)},
			wantPercent: iptr(25),
			wantBasis:   "docs_estimated",
			wantDocs:    25,
		},
		{
			name:        "batches fallback when no estimate",
			attempt:     ragmodel.RAGIndexAttempt{Status: ragmodel.IndexingStatusInProgress, TotalDocsIndexed: iptr(3), TotalBatches: iptr(4), CompletedBatches: 1},
			wantPercent: iptr(25),
			wantBasis:   "batches",
			wantDocs:    3,
		},
		{
			name:        "no denominator is indeterminate",
			attempt:     ragmodel.RAGIndexAttempt{Status: ragmodel.IndexingStatusInProgress, TotalDocsIndexed: iptr(42)},
			wantPercent: nil,
			wantBasis:   "",
			wantIndet:   true,
			wantDocs:    42,
		},
		{
			name:        "docs indexed over estimate clamps to 100",
			attempt:     ragmodel.RAGIndexAttempt{Status: ragmodel.IndexingStatusInProgress, TotalDocsIndexed: iptr(150), DocsEstimated: iptr(100)},
			wantPercent: iptr(100),
			wantBasis:   "docs_estimated",
			wantDocs:    150,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := computeRAGProgress(&tc.attempt)
			if got.Indeterminate != tc.wantIndet {
				t.Fatalf("indeterminate = %v, want %v", got.Indeterminate, tc.wantIndet)
			}
			if got.Basis != tc.wantBasis {
				t.Fatalf("basis = %q, want %q", got.Basis, tc.wantBasis)
			}
			if got.DocsIndexed != tc.wantDocs {
				t.Fatalf("docs_indexed = %d, want %d", got.DocsIndexed, tc.wantDocs)
			}
			switch {
			case tc.wantPercent == nil && got.Percent != nil:
				t.Fatalf("percent = %d, want nil", *got.Percent)
			case tc.wantPercent != nil && got.Percent == nil:
				t.Fatalf("percent = nil, want %d", *tc.wantPercent)
			case tc.wantPercent != nil && *got.Percent != *tc.wantPercent:
				t.Fatalf("percent = %d, want %d", *got.Percent, *tc.wantPercent)
			}
		})
	}
}
