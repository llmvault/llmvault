package website

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
	"github.com/usehivy/hivy/internal/spider"
)

func (c *WebsiteConnector) Run(
	ctx context.Context,
	_ interfaces.Source,
	_ json.RawMessage,
	_, _ time.Time,
) (<-chan interfaces.DocumentOrFailure, error) {
	out := make(chan interfaces.DocumentOrFailure, 32)
	respect := true
	if c.cfg.RespectRobots != nil {
		respect = *c.cfg.RespectRobots
	}

	go func() {
		defer close(out)
		// One crawl per seed URL, path-limited to its own subtree, under a
		// shared page budget so a source with many seeds can't blow past MaxPages.
		remaining := c.cfg.MaxPages
		if remaining <= 0 {
			remaining = defaultMaxPages
		}
		for _, seed := range c.seeds() {
			if ctx.Err() != nil || remaining <= 0 {
				return
			}
			limit := remaining
			stream, errs := c.spider.CrawlStream(ctx, spider.SpiderParams{
				URL:           seed,
				Whitelist:     seedWhitelist(seed),
				ReturnFormat:  "markdown",
				RequestType:   "smart",
				Readability:   ptr(true),
				Sitemap:       ptr(true),
				RespectRobots: ptr(respect),
				Limit:         &limit,
			})
			remaining -= c.drainSeed(ctx, seed, stream, errs, out)
		}
	}()

	return out, nil
}

// drainSeed forwards one seed crawl's pages into out and returns how many pages
// it consumed from the budget (every page fetched, success or failure).
func (c *WebsiteConnector) drainSeed(
	ctx context.Context,
	seed string,
	stream <-chan spider.Response,
	errs <-chan error,
	out chan<- interfaces.DocumentOrFailure,
) int {
	consumed := 0
	for {
		select {
		case r, ok := <-stream:
			if !ok {
				if err, ok := <-errs; ok && err != nil {
					out <- interfaces.NewDocFailure(streamFailure(seed, err))
				}
				return consumed
			}
			consumed++
			if r.Error != "" || (r.StatusCode != 0 && r.StatusCode >= 400) {
				out <- interfaces.NewDocFailure(pageFailure(r))
				continue
			}
			if strings.TrimSpace(r.Content) == "" {
				continue
			}
			doc := responseToDocument(r)
			out <- interfaces.NewDocResult(&doc)
		case <-ctx.Done():
			return consumed
		}
	}
}

// seeds returns the crawl seeds, falling back to the legacy single URL for
// configs built without LoadConfig (e.g. tests).
func (c *WebsiteConnector) seeds() []string {
	if len(c.cfg.URLs) > 0 {
		return c.cfg.URLs
	}
	if s := strings.TrimSpace(c.cfg.URL); s != "" {
		return []string{s}
	}
	return nil
}

// FinalCheckpoint always returns nil — v1 has no resume; a failed crawl
// restarts from the seed URLs on the next attempt.
func (c *WebsiteConnector) FinalCheckpoint() (json.RawMessage, error) {
	return nil, nil
}

func pageFailure(r spider.Response) *interfaces.ConnectorFailure {
	msg := r.Error
	if msg == "" {
		msg = "spider returned non-2xx status"
	}
	return interfaces.NewDocumentFailure(canonicalURL(r.URL), r.URL, msg, nil)
}

func streamFailure(seed string, err error) *interfaces.ConnectorFailure {
	return interfaces.NewEntityFailure(seed, err.Error(), nil, nil, err)
}

func ptr[T any](v T) *T { return &v }
