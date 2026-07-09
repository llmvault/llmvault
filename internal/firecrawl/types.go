package firecrawl

// ScrapeParams is the POST /v2/scrape request body.
type ScrapeParams struct {
	URL             string   `json:"url"`
	Formats         []string `json:"formats"`
	OnlyMainContent bool     `json:"onlyMainContent"`
	Timeout         *int     `json:"timeout,omitempty"`
}

// Metadata is the subset of scrape/crawl item metadata this client decodes.
// title/description are intentionally omitted because Firecrawl returns them as
// either a string or an array; only these four fields are relied upon.
type Metadata struct {
	SourceURL  string `json:"sourceURL"`
	URL        string `json:"url"`
	StatusCode int    `json:"statusCode"`
	Error      string `json:"error"`
}

// ScrapeData is the "data" object of a scrape response, also reused as a crawl
// data item.
type ScrapeData struct {
	Markdown string   `json:"markdown"`
	HTML     string   `json:"html"`
	RawHTML  string   `json:"rawHtml"`
	Links    []string `json:"links"`
	Metadata Metadata `json:"metadata"`
	Warning  string   `json:"warning"`
}

// ScrapeResponse is the top-level POST /v2/scrape response.
type ScrapeResponse struct {
	Success bool       `json:"success"`
	Data    ScrapeData `json:"data"`
	Error   string     `json:"error"`
	Code    string     `json:"code"`
}

// SearchParams is the POST /v2/search request body.
type SearchParams struct {
	Query string `json:"query"`
	Limit *int   `json:"limit,omitempty"`
}

// WebResult is a single web search hit.
type WebResult struct {
	Title       string `json:"title"`
	Description string `json:"description"`
	URL         string `json:"url"`
}

// SearchResponse is the top-level POST /v2/search response.
type SearchResponse struct {
	Success bool `json:"success"`
	Data    struct {
		Web []WebResult `json:"web"`
	} `json:"data"`
	CreditsUsed int    `json:"creditsUsed"`
	Error       string `json:"error"`
	Code        string `json:"code"`
}

// CrawlParams is the POST /v2/crawl request body.
type CrawlParams struct {
	URL               string             `json:"url"`
	Limit             *int               `json:"limit,omitempty"`
	MaxDiscoveryDepth *int               `json:"maxDiscoveryDepth,omitempty"`
	ScrapeOptions     *CrawlScrapeOption `json:"scrapeOptions,omitempty"`
}

// CrawlScrapeOption configures per-page scraping during a crawl.
type CrawlScrapeOption struct {
	Formats []string `json:"formats"`
}

// StartCrawlResponse is the top-level POST /v2/crawl response.
type StartCrawlResponse struct {
	Success bool   `json:"success"`
	ID      string `json:"id"`
	URL     string `json:"url"`
	Error   string `json:"error"`
	Code    string `json:"code"`
}

// CrawlStatusResponse is the top-level GET /v2/crawl/{id} response.
type CrawlStatusResponse struct {
	Status      string       `json:"status"`
	Total       int          `json:"total"`
	Completed   int          `json:"completed"`
	CreditsUsed int          `json:"creditsUsed"`
	ExpiresAt   string       `json:"expiresAt"`
	Next        string       `json:"next"`
	Data        []ScrapeData `json:"data"`
	Success     bool         `json:"success"`
	Error       string       `json:"error"`
	Code        string       `json:"code"`
}

// MapParams is the POST /v2/map request body.
type MapParams struct {
	URL               string `json:"url"`
	Limit             *int   `json:"limit,omitempty"`
	IncludeSubdomains *bool  `json:"includeSubdomains,omitempty"`
}

// MapLink is a single link returned by POST /v2/map.
type MapLink struct {
	URL         string `json:"url"`
	Title       string `json:"title"`
	Description string `json:"description"`
}

// MapResponse is the top-level POST /v2/map response.
type MapResponse struct {
	Success bool      `json:"success"`
	Links   []MapLink `json:"links"`
	Error   string    `json:"error"`
	Code    string    `json:"code"`
}
