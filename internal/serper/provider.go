package serper

import (
	"context"
	"errors"

	"github.com/usehivy/hivy/internal/webcrawl"
)

// Provider adapts a Client to the webcrawl.Provider contract. Serper only
// does web search; the other operations report unsupported and must not be
// routed to this provider.
type Provider struct {
	client *Client
}

var _ webcrawl.Provider = (*Provider)(nil)

// NewProvider wraps a Client as a webcrawl.Provider.
func NewProvider(client *Client) *Provider {
	return &Provider{client: client}
}

// Name identifies this provider.
func (provider *Provider) Name() string { return "serper" }

// maxNum is Serper's per-page result cap; each block of 10 results costs one
// credit, so num is also the cost knob.
const maxNum = 100

// Search performs a web search. Zero hits is a valid empty result.
func (provider *Provider) Search(ctx context.Context, req webcrawl.SearchRequest) ([]webcrawl.SearchResult, error) {
	params := SearchParams{Query: req.Query}
	if req.Limit > 0 {
		num := min(req.Limit, maxNum)
		params.Num = &num
	}
	results, err := provider.client.Search(ctx, params)
	if err != nil {
		return nil, err
	}
	out := make([]webcrawl.SearchResult, 0, len(results))
	for _, result := range results {
		out = append(out, webcrawl.SearchResult{
			URL:         result.Link,
			Title:       result.Title,
			Description: result.Snippet,
		})
	}
	return out, nil
}

func (provider *Provider) Scrape(context.Context, webcrawl.ScrapeRequest) (webcrawl.Page, error) {
	return webcrawl.Page{}, errors.New("serper: scrape not supported")
}

func (provider *Provider) Crawl(context.Context, webcrawl.CrawlRequest) ([]webcrawl.Page, error) {
	return nil, errors.New("serper: crawl not supported")
}

func (provider *Provider) Map(context.Context, webcrawl.MapRequest) ([]string, error) {
	return nil, errors.New("serper: map not supported")
}
