// Package webcrawl defines the provider-agnostic contract for web search,
// fetch, crawl, and site mapping, plus a fallback chain that composes
// multiple providers. Concrete implementations live in internal/spider and
// internal/firecrawl.
package webcrawl

import "context"

// Format is the requested output format for page content.
type Format string

const (
	FormatMarkdown Format = "markdown"
	FormatText     Format = "text"
	FormatHTML     Format = "html"
)

// Page is a single fetched page.
type Page struct {
	URL        string
	Content    string
	StatusCode int
}

// ScrapeRequest fetches a single page. Zero-value Format means markdown.
type ScrapeRequest struct {
	URL    string
	Format Format
}

// CrawlRequest fetches up to Limit pages starting from URL, following links.
// Zero values mean provider defaults.
type CrawlRequest struct {
	URL    string
	Limit  int
	Depth  int
	Format Format
}

// SearchRequest performs a web search. Zero Limit means provider default.
type SearchRequest struct {
	Query string
	Limit int
}

// SearchResult is a single web search hit.
type SearchResult struct {
	URL         string `json:"url,omitempty"`
	Title       string `json:"title,omitempty"`
	Description string `json:"description,omitempty"`
}

// MapRequest enumerates URLs reachable from URL for site discovery.
// Zero Limit means provider default.
type MapRequest struct {
	URL   string
	Limit int
}

// Provider is a web data provider (Spider.cloud, Firecrawl, ...).
// Implementations must return an error — never a silently empty result —
// when the upstream call fails or yields nothing usable, so a fallback
// chain can move on to the next provider.
type Provider interface {
	Name() string
	Scrape(ctx context.Context, req ScrapeRequest) (Page, error)
	Crawl(ctx context.Context, req CrawlRequest) ([]Page, error)
	Search(ctx context.Context, req SearchRequest) ([]SearchResult, error)
	Map(ctx context.Context, req MapRequest) ([]string, error)
}
