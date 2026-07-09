package spider

import (
	"context"
	"fmt"

	"github.com/usehivy/hivy/internal/webcrawl"
)

// Provider adapts a Spider.cloud *Client to the webcrawl.Provider interface.
type Provider struct {
	client *Client
}

// NewProvider wraps a Spider.cloud client as a webcrawl.Provider.
func NewProvider(client *Client) *Provider {
	return &Provider{client: client}
}

// Name reports the provider identity.
func (p *Provider) Name() string { return "spider" }

// returnFormat maps a webcrawl.Format to Spider.cloud's return_format value.
func returnFormat(format webcrawl.Format) string {
	switch format {
	case webcrawl.FormatText:
		return "text"
	case webcrawl.FormatHTML:
		return "raw"
	default:
		return "markdown"
	}
}

// Scrape fetches a single page via /v1/crawl with limit 1.
func (p *Provider) Scrape(ctx context.Context, req webcrawl.ScrapeRequest) (webcrawl.Page, error) {
	limit := 1
	params := SpiderParams{
		URL:          req.URL,
		Limit:        &limit,
		ReturnFormat: returnFormat(req.Format),
		RequestType:  "smart",
	}
	if params.ReturnFormat != "raw" {
		params.Readability = boolPtr(true)
	}

	results, err := p.client.Crawl(ctx, params)
	if err != nil {
		return webcrawl.Page{}, err
	}
	if len(results) == 0 {
		return webcrawl.Page{}, fmt.Errorf("spider scrape: empty response for %s", req.URL)
	}

	r := results[0]
	if r.Error != "" {
		return webcrawl.Page{}, fmt.Errorf("spider scrape: %s for %s", r.Error, req.URL)
	}
	if r.HTTPStatus() >= 400 {
		return webcrawl.Page{}, fmt.Errorf("spider scrape: status %d for %s", r.HTTPStatus(), req.URL)
	}

	url := r.URL
	if url == "" {
		url = req.URL
	}
	return webcrawl.Page{URL: url, Content: r.Content, StatusCode: r.HTTPStatus()}, nil
}

// Crawl fetches multiple pages via /v1/crawl, skipping per-page errors.
func (p *Provider) Crawl(ctx context.Context, req webcrawl.CrawlRequest) ([]webcrawl.Page, error) {
	params := SpiderParams{
		URL:          req.URL,
		ReturnFormat: returnFormat(req.Format),
		RequestType:  "smart",
	}
	if req.Limit > 0 {
		params.Limit = &req.Limit
	}
	if req.Depth > 0 {
		params.Depth = &req.Depth
	}

	results, err := p.client.Crawl(ctx, params)
	if err != nil {
		return nil, err
	}

	pages := make([]webcrawl.Page, 0, len(results))
	for _, r := range results {
		if r.Error != "" {
			continue
		}
		url := r.URL
		if url == "" {
			url = req.URL
		}
		pages = append(pages, webcrawl.Page{URL: url, Content: r.Content, StatusCode: r.HTTPStatus()})
	}
	if len(pages) == 0 {
		return nil, fmt.Errorf("spider crawl: no usable pages for %s", req.URL)
	}
	return pages, nil
}

// Search performs a web search via /v1/search.
func (p *Provider) Search(ctx context.Context, req webcrawl.SearchRequest) ([]webcrawl.SearchResult, error) {
	params := SearchParams{
		SpiderParams: SpiderParams{RequestType: "smart"},
		Search:       req.Query,
	}
	if req.Limit > 0 {
		params.Num = &req.Limit
		params.SearchLimit = &req.Limit
	}

	resp, err := p.client.Search(ctx, params)
	if err != nil {
		return nil, err
	}

	results := make([]webcrawl.SearchResult, 0, len(resp.Content))
	for _, r := range resp.Content {
		results = append(results, webcrawl.SearchResult{
			URL:         r.URL,
			Title:       r.Title,
			Description: r.Description,
		})
	}
	return results, nil
}

// Map enumerates URLs reachable from req.URL via /v1/links.
func (p *Provider) Map(ctx context.Context, req webcrawl.MapRequest) ([]string, error) {
	params := SpiderParams{
		URL:             req.URL,
		RequestType:     "smart",
		ReturnPageLinks: boolPtr(true),
		RespectRobots:   boolPtr(true),
	}
	if req.Limit > 0 {
		params.Limit = &req.Limit
	}

	results, err := p.client.Links(ctx, params)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]struct{})
	urls := make([]string, 0)
	add := func(u string) {
		if u == "" {
			return
		}
		if _, ok := seen[u]; ok {
			return
		}
		seen[u] = struct{}{}
		urls = append(urls, u)
	}
	for _, r := range results {
		add(r.URL)
		for _, link := range r.Links {
			add(link)
		}
	}
	if len(urls) == 0 {
		return nil, fmt.Errorf("spider map: no URLs for %s", req.URL)
	}
	return urls, nil
}

var _ webcrawl.Provider = (*Provider)(nil)
