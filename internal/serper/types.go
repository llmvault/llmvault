package serper

// SearchParams is the POST body for /search.
type SearchParams struct {
	Query string `json:"q"`
	Num   *int   `json:"num,omitempty"`
}

// OrganicResult is a single organic search hit.
type OrganicResult struct {
	Title    string `json:"title"`
	Link     string `json:"link"`
	Snippet  string `json:"snippet"`
	Position int    `json:"position"`
}

// SearchResponse is the top-level response from /search. Serper also returns
// knowledgeGraph/relatedSearches/searchParameters blocks; only organic is
// decoded.
type SearchResponse struct {
	Organic []OrganicResult `json:"organic"`
}

type errorResponse struct {
	Message    string `json:"message"`
	StatusCode int    `json:"statusCode"`
}
