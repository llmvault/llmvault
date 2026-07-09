package website

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/goroutine"
	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
	"github.com/usehivy/hivy/internal/webcrawl"
)

func (c *WebsiteConnector) Run(
	ctx context.Context,
	_ interfaces.Source,
	_ json.RawMessage,
	_, _ time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	out := make(chan interfaces.DocumentOrFailure, 32)

	goroutine.Go(ctx, func(ctx context.Context) {
		defer close(out)
		// Scrape each configured URL to markdown; MaxPages caps the list.
		max := c.cfg.MaxPages
		if max <= 0 {
			max = defaultMaxPages
		}
		for i, pageURL := range c.urls() {
			if ctx.Err() != nil || i >= max {
				return
			}
			r, err := c.web.Scrape(ctx, webcrawl.ScrapeRequest{URL: pageURL})
			if err != nil {
				if !interfaces.Send(ctx, out, interfaces.NewDocFailure(scrapeFailure(pageURL, err))) {
					return
				}
				continue
			}
			if strings.TrimSpace(r.Content) == "" {
				continue
			}
			if r.URL == "" {
				r.URL = pageURL
			}
			doc := responseToDocument(r)
			if !interfaces.Send(ctx, out, interfaces.NewDocResult(&doc)) {
				return
			}
		}
	})

	return out, nil
}

// urls returns the pages to scrape, falling back to the legacy single URL for
// configs built without LoadConfig (e.g. tests).
func (c *WebsiteConnector) urls() []string {
	if len(c.cfg.URLs) > 0 {
		return c.cfg.URLs
	}
	if s := strings.TrimSpace(c.cfg.URL); s != "" {
		return []string{s}
	}
	return nil
}

// FinalCheckpoint always returns nil — v1 has no resume; a failed run restarts
// from the configured URLs on the next attempt.
func (c *WebsiteConnector) FinalCheckpoint() (json.RawMessage, error) {
	return nil, nil
}

func scrapeFailure(pageURL string, err error) *interfaces.ConnectorFailure {
	return interfaces.NewDocumentFailure(canonicalURL(pageURL), pageURL, "website: scrape failed", err)
}
