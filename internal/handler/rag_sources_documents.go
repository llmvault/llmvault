package handler

import (
	"context"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	qc "github.com/qdrant/go-client/qdrant"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/rag/qdrant"
)

type ragDocument struct {
	DocID        string `json:"doc_id"`
	Title        string `json:"title,omitempty"`
	Link         string `json:"link,omitempty"`
	DocUpdatedAt *int64 `json:"doc_updated_at,omitempty"`
}

type ragDocumentsResponse struct {
	Documents  []ragDocument `json:"documents"`
	NextCursor string        `json:"next_cursor,omitempty"`
}

// ListDocuments handles GET /v1/rag/sources/{id}/documents.
//
// Documents live only in Qdrant; this is a thin passthrough that scrolls the
// vector store filtered to the source and returns one row per document. Each
// document is anchored on its first chunk (part_index == 0), so a multi-chunk
// document lists exactly once and Qdrant's native scroll offset paginates
// cleanly at the document level. Results are org-scoped by the org_id filter,
// so a source id from another org simply returns nothing.
//
// @Summary List documents ingested for a RAG source
// @Description Lists the documents ingested into the knowledge base for a source, read directly from the vector store. Paginated via an opaque cursor.
// @Tags rag
// @Produce json
// @Param id path string true "RAG source ID"
// @Param limit query int false "Max documents to return (default 50, max 200)"
// @Param cursor query string false "Opaque pagination cursor from a previous response"
// @Success 200 {object} ragDocumentsResponse
// @Security BearerAuth
// @Router /v1/rag/sources/{id}/documents [get]
func (h *RAGSearchHandler) ListDocuments(w http.ResponseWriter, r *http.Request) {
	if h.qd == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "rag search not configured"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}
	sourceID := chi.URLParam(r, "id")
	if sourceID == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "source id required"})
		return
	}

	limit := 50
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if limit > 200 {
		limit = 200
	}

	var offset *qc.PointId
	if cursor := r.URL.Query().Get("cursor"); cursor != "" {
		offset = qc.NewID(cursor)
	}

	// org_id + rag_source_id + part_index == 0 → one representative point per doc.
	filter := &qc.Filter{Must: []*qc.Condition{
		qc.NewMatchKeyword("org_id", org.ID.String()),
		qc.NewMatchKeyword("rag_source_id", sourceID),
		qc.NewMatchInt("part_index", 0),
	}}

	ctx, cancel := context.WithTimeout(r.Context(), 20*time.Second)
	defer cancel()

	page, err := h.qd.Scroll(ctx, qdrant.ScrollRequest{
		Collection:  h.collection,
		Filter:      filter,
		Limit:       uint32(limit), // #nosec G115 -- limit is clamped to 1..200 above.
		Offset:      offset,
		WithPayload: true,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "qdrant scroll: " + err.Error()})
		return
	}

	out := ragDocumentsResponse{Documents: make([]ragDocument, 0, len(page.Points))}
	for _, p := range page.Points {
		doc := ragDocument{}
		if p.Payload != nil {
			if v, ok := p.Payload["doc_id"].(string); ok {
				doc.DocID = v
			}
			if v, ok := p.Payload["semantic_id"].(string); ok {
				doc.Title = v
			}
			if v, ok := p.Payload["link"].(string); ok {
				doc.Link = v
			}
			if ts, ok := payloadInt64(p.Payload["doc_updated_at"]); ok {
				doc.DocUpdatedAt = &ts
			}
		}
		out.Documents = append(out.Documents, doc)
	}
	if page.NextOffset != nil {
		out.NextCursor = page.NextOffset.GetUuid()
	}
	writeJSON(w, http.StatusOK, out)
}

func payloadInt64(v any) (int64, bool) {
	switch n := v.(type) {
	case int64:
		return n, true
	case int:
		return int64(n), true
	case float64:
		return int64(n), true
	default:
		return 0, false
	}
}
