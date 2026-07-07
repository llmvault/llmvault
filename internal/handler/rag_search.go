package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/usehivy/hivy/internal/middleware"
	"github.com/usehivy/hivy/internal/rag/embedclient"
	"github.com/usehivy/hivy/internal/rag/qdrant"
)

type RAGSearchHandler struct {
	db         *gorm.DB
	qd         *qdrant.Client
	embedder   *embedclient.Embedder
	reranker   *embedclient.Reranker
	collection string
}

func NewRAGSearchHandler(db *gorm.DB, qd *qdrant.Client, embedder *embedclient.Embedder,
	reranker *embedclient.Reranker, collection string) *RAGSearchHandler {
	return &RAGSearchHandler{db: db, qd: qd, embedder: embedder, reranker: reranker, collection: collection}
}

type ragSearchRequest struct {
	Query     string   `json:"query"`
	Rerank    bool     `json:"rerank,omitempty"`
	Limit     uint32   `json:"limit,omitempty"`
	SourceIDs []string `json:"source_ids,omitempty"`
}

type ragSearchHit struct {
	ID          string  `json:"id"`
	DocID       string  `json:"doc_id"`
	Score       float64 `json:"score"`
	RerankScore float64 `json:"rerank_score,omitempty"`
	Title       string  `json:"title,omitempty"`
	Link        string  `json:"link,omitempty"`
	Blurb       string  `json:"blurb,omitempty"`
	Content     string  `json:"content,omitempty"`
}

type ragSearchResponse struct {
	Hits []ragSearchHit `json:"hits"`
}

const candidatePool uint32 = 50

// @Summary Search the knowledge base
// @Description Hybrid retrieval against the org's RAG dataset, optionally reranked.
// @Tags rag
// @Accept json
// @Produce json
// @Param body body ragSearchRequest true "Search query"
// @Success 200 {object} ragSearchResponse
// @Security BearerAuth
// @Router /v1/rag/search [post]
func (h *RAGSearchHandler) Search(w http.ResponseWriter, r *http.Request) {
	if h.qd == nil || h.embedder == nil {
		writeJSON(w, http.StatusServiceUnavailable, errorResponse{Error: "rag search not configured"})
		return
	}
	org, ok := middleware.OrgFromContext(r.Context())
	if !ok {
		writeJSON(w, http.StatusUnauthorized, errorResponse{Error: "missing org context"})
		return
	}

	var req ragSearchRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "invalid request body"})
		return
	}
	if strings.TrimSpace(req.Query) == "" {
		writeJSON(w, http.StatusBadRequest, errorResponse{Error: "query is required"})
		return
	}
	limit := req.Limit
	if limit == 0 || limit > 50 {
		limit = 10
	}

	// Channel-scope the search for non-manager members: they may only reach
	// sources granted to channels they can use. Managers and API-key callers
	// keep org-wide reach. Any caller-supplied source_ids are intersected with
	// the usable set so a member cannot widen their scope by asking.
	sourceIDs := req.SourceIDs
	orgWide, userID, err := actorSeesOrgWide(r.Context(), h.db, org.ID)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve access"})
		return
	}
	if !orgWide {
		usable, err := usableRagSourceIDs(r.Context(), h.db, org.ID, userID)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, errorResponse{Error: "failed to resolve knowledge access"})
			return
		}
		sourceIDs = intersectSourceIDs(usable, req.SourceIDs)
		// Deny-by-default: no usable source means no results.
		if len(sourceIDs) == 0 {
			writeJSON(w, http.StatusOK, ragSearchResponse{Hits: []ragSearchHit{}})
			return
		}
	}

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()

	vectors, err := h.embedder.Embed(ctx, []string{req.Query})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "embed query: " + err.Error()})
		return
	}

	filter := qdrant.BuildScopedFilter(org.ID.String(), sourceIDs)

	topK := limit
	if req.Rerank && h.reranker != nil {
		topK = candidatePool
	}
	hits, err := h.qd.Search(ctx, qdrant.SearchRequest{
		Collection:  h.collection,
		Vector:      vectors[0],
		Filter:      filter,
		Limit:       topK,
		WithPayload: true,
	})
	if err != nil {
		writeJSON(w, http.StatusBadGateway, errorResponse{Error: "qdrant search: " + err.Error()})
		return
	}

	out := ragSearchResponse{Hits: hitsToResponse(hits)}
	if req.Rerank && h.reranker != nil && len(hits) > 0 {
		reranked, err := rerankHits(ctx, h.reranker, req.Query, out.Hits, int(limit))
		if err != nil {
			writeJSON(w, http.StatusBadGateway, errorResponse{Error: "rerank: " + err.Error()})
			return
		}
		out.Hits = reranked
	} else if len(out.Hits) > int(limit) {
		out.Hits = out.Hits[:limit]
	}
	writeJSON(w, http.StatusOK, out)
}

// intersectSourceIDs restricts the caller-requested source ids to the usable
// set. An empty request means "all usable", so it returns the full usable set.
func intersectSourceIDs(usable, requested []string) []string {
	if len(requested) == 0 {
		return usable
	}
	allowed := make(map[string]struct{}, len(usable))
	for _, id := range usable {
		allowed[id] = struct{}{}
	}
	out := make([]string, 0, len(requested))
	for _, id := range requested {
		if _, ok := allowed[id]; ok {
			out = append(out, id)
		}
	}
	return out
}

func hitsToResponse(hits []qdrant.Hit) []ragSearchHit {
	out := make([]ragSearchHit, 0, len(hits))
	for _, h := range hits {
		hit := ragSearchHit{ID: h.ID, Score: h.Score}
		if h.Payload != nil {
			if v, ok := h.Payload["doc_id"].(string); ok {
				hit.DocID = v
			}
			if v, ok := h.Payload["semantic_id"].(string); ok {
				hit.Title = v
			}
			if v, ok := h.Payload["link"].(string); ok {
				hit.Link = v
			}
			if v, ok := h.Payload["content"].(string); ok {
				hit.Content = v
				if len(v) > 200 {
					hit.Blurb = v[:200]
				} else {
					hit.Blurb = v
				}
			}
		}
		out = append(out, hit)
	}
	return out
}

func rerankHits(ctx context.Context, rer *embedclient.Reranker, query string,
	hits []ragSearchHit, topN int) ([]ragSearchHit, error) {
	docs := make([]string, len(hits))
	for i := range hits {
		c := hits[i].Content
		if len(c) > 1500 {
			c = c[:1500]
		}
		docs[i] = c
	}
	results, err := rer.Rerank(ctx, query, docs, topN)
	if err != nil {
		return nil, err
	}
	out := make([]ragSearchHit, 0, len(results))
	for _, r := range results {
		if r.Index < 0 || r.Index >= len(hits) {
			continue
		}
		hit := hits[r.Index]
		hit.RerankScore = r.Score
		out = append(out, hit)
	}
	return out, nil
}
