package qdrant

import (
	"context"

	qc "github.com/qdrant/go-client/qdrant"
)

type Filter = qc.Filter

type SearchRequest struct {
	Collection  string
	Vector      []float32
	Filter      *qc.Filter
	Limit       uint32
	HNSWEf      uint32
	WithPayload bool
}

type Hit struct {
	ID      string
	Score   float64
	Payload map[string]any
}

func (c *Client) Search(ctx context.Context, req SearchRequest) ([]Hit, error) {
	limit := req.Limit
	if limit == 0 {
		limit = 10
	}
	q := &qc.QueryPoints{
		CollectionName: req.Collection,
		Query:          qc.NewQueryDense(req.Vector),
		Filter:         req.Filter,
		Limit:          qc.PtrOf(uint64(limit)),
		WithPayload:    qc.NewWithPayload(req.WithPayload),
	}
	if req.HNSWEf > 0 {
		q.Params = &qc.SearchParams{HnswEf: qc.PtrOf(uint64(req.HNSWEf))}
	}
	pts, err := c.c.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	out := make([]Hit, len(pts))
	for i, p := range pts {
		out[i] = Hit{
			ID:      pointIDString(p.GetId()),
			Score:   float64(p.GetScore()),
			Payload: fromValueMap(p.GetPayload()),
		}
	}
	return out, nil
}

// BuildOrgFilter partitions results to a single org: org_id == orgID. This is
// the base tenant isolation every RAG query carries.
func BuildOrgFilter(orgID string) *qc.Filter {
	return &qc.Filter{Must: []*qc.Condition{qc.NewMatchKeyword("org_id", orgID)}}
}

// org_id == X AND rag_source_id == Y.
func BuildSourceFilter(orgID, sourceID string) *qc.Filter {
	return &qc.Filter{Must: []*qc.Condition{
		qc.NewMatchKeyword("org_id", orgID),
		qc.NewMatchKeyword("rag_source_id", sourceID),
	}}
}

// BuildScopedFilter partitions by org and, when sourceIDs is non-empty, further
// restricts results to that set of sources via a mandatory
// rag_source_id any-of [sourceIDs] clause. When sourceIDs is empty the filter is
// identical to BuildOrgFilter.
func BuildScopedFilter(orgID string, sourceIDs []string) *qc.Filter {
	filter := BuildOrgFilter(orgID)
	if len(sourceIDs) > 0 {
		filter.Must = append(filter.Must, qc.NewMatchKeywords("rag_source_id", sourceIDs...))
	}
	return filter
}

func pointIDString(id *qc.PointId) string {
	if id == nil {
		return ""
	}
	if u := id.GetUuid(); u != "" {
		return u
	}
	if n := id.GetNum(); n != 0 {
		return ""
	}
	return ""
}
