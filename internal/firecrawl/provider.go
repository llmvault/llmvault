package firecrawl

import (
	"context"
	"fmt"
	"time"

	"github.com/usehivy/hivy/internal/webcrawl"
)

// crawlDeadline caps how long Provider.Crawl polls for a crawl to complete.
const crawlDeadline = 5 * time.Minute

// Provider adapts a Client to the webcrawl.Provider contract.
type Provider struct {
	client *Client
}

var _ webcrawl.Provider = (*Provider)(nil)

// NewProvider wraps a Client as a webcrawl.Provider.
func NewProvider(client *Client) *Provider {
	return &Provider{client: client}
}

// Name identifies this provider.
func (provider *Provider) Name() string { return "firecrawl" }

// Scrape fetches a single page.
func (provider *Provider) Scrape(ctx context.Context, req webcrawl.ScrapeRequest) (webcrawl.Page, error) {
	data, err := provider.client.Scrape(ctx, ScrapeParams{
		URL:             req.URL,
		Formats:         formatsFor(req.Format),
		OnlyMainContent: true,
	})
	if err != nil {
		return webcrawl.Page{}, err
	}
	if data.Metadata.Error != "" {
		return webcrawl.Page{}, fmt.Errorf("firecrawl scrape %s: %s", req.URL, data.Metadata.Error)
	}
	if data.Metadata.StatusCode >= 400 {
		return webcrawl.Page{}, fmt.Errorf("firecrawl scrape %s: status %d", req.URL, data.Metadata.StatusCode)
	}
	content := contentFor(*data, req.Format)
	if content == "" {
		return webcrawl.Page{}, fmt.Errorf("firecrawl scrape %s: empty content", req.URL)
	}
	return webcrawl.Page{
		URL:        pageURL(data.Metadata, req.URL),
		Content:    content,
		StatusCode: data.Metadata.StatusCode,
	}, nil
}

// Crawl starts a crawl, polls until it completes, and returns its pages.
func (provider *Provider) Crawl(ctx context.Context, req webcrawl.CrawlRequest) ([]webcrawl.Page, error) {
	params := CrawlParams{
		URL:           req.URL,
		ScrapeOptions: &CrawlScrapeOption{Formats: formatsFor(req.Format)},
	}
	if req.Limit > 0 {
		params.Limit = &req.Limit
	}
	if req.Depth > 0 {
		params.MaxDiscoveryDepth = &req.Depth
	}

	id, err := provider.client.StartCrawl(ctx, params)
	if err != nil {
		return nil, err
	}

	pollCtx, cancel := context.WithTimeout(ctx, crawlDeadline)
	defer cancel()

	statusURL := provider.client.CrawlStatusURL(id)
	ticker := time.NewTicker(provider.client.pollInterval)
	defer ticker.Stop()

	for {
		status, err := provider.client.CrawlStatus(pollCtx, statusURL)
		if err != nil {
			return nil, err
		}
		switch status.Status {
		case "completed":
			return provider.collectCrawl(pollCtx, status, req)
		case "scraping", "cancelling":
		default:
			return nil, fmt.Errorf("firecrawl crawl %s: status %s", req.URL, status.Status)
		}
		select {
		case <-pollCtx.Done():
			return nil, pollCtx.Err()
		case <-ticker.C:
		}
	}
}

func (provider *Provider) collectCrawl(ctx context.Context, status *CrawlStatusResponse, req webcrawl.CrawlRequest) ([]webcrawl.Page, error) {
	items := append([]ScrapeData(nil), status.Data...)
	next := status.Next
	for next != "" {
		page, err := provider.client.CrawlStatus(ctx, next)
		if err != nil {
			return nil, err
		}
		items = append(items, page.Data...)
		next = page.Next
	}

	pages := make([]webcrawl.Page, 0, len(items))
	for _, item := range items {
		if item.Metadata.Error != "" {
			continue
		}
		content := contentFor(item, req.Format)
		if content == "" {
			continue
		}
		pages = append(pages, webcrawl.Page{
			URL:        pageURL(item.Metadata, req.URL),
			Content:    content,
			StatusCode: item.Metadata.StatusCode,
		})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("firecrawl crawl %s: no usable pages", req.URL)
	}
	return pages, nil
}

// Search performs a web search. Zero hits is a valid empty result.
func (provider *Provider) Search(ctx context.Context, req webcrawl.SearchRequest) ([]webcrawl.SearchResult, error) {
	params := SearchParams{Query: req.Query}
	if req.Limit > 0 {
		params.Limit = &req.Limit
	}
	results, err := provider.client.Search(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]webcrawl.SearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, webcrawl.SearchResult{
			URL:         result.URL,
			Title:       result.Title,
			Description: result.Description,
		})
	}
	return out, nil
}

// Map returns the deduplicated URLs reachable from a site. Firecrawl defaults
// includeSubdomains to true; it is pinned false so map stays host-scoped like
// the other providers.
func (provider *Provider) Map(ctx context.Context, req webcrawl.MapRequest) ([]string, error) {
	includeSubdomains := false
	params := MapParams{URL: req.URL, IncludeSubdomains: &includeSubdomains}
	if req.Limit > 0 {
		params.Limit = &req.Limit
	}
	links, err := provider.client.Map(ctx, params)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{}, len(links))
	out := make([]string, 0, len(links))
	for _, link := range links {
		if link.URL == "" {
			continue
		}
		if _, ok := seen[link.URL]; ok {
			continue
		}
		seen[link.URL] = struct{}{}
		out = append(out, link.URL)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("firecrawl map %s: no links", req.URL)
	}
	return out, nil
}

func formatsFor(format webcrawl.Format) []string {
	if format == webcrawl.FormatHTML {
		return []string{"html"}
	}
	return []string{"markdown"}
}

func contentFor(data ScrapeData, format webcrawl.Format) string {
	if format == webcrawl.FormatHTML {
		return data.HTML
	}
	return data.Markdown
}

func pageURL(meta Metadata, fallback string) string {
	if meta.SourceURL != "" {
		return meta.SourceURL
	}
	if meta.URL != "" {
		return meta.URL
	}
	return fallback
}
