package bootstrap

import (
	"time"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/firecrawl"
	"github.com/usehivy/hivy/internal/serper"
	"github.com/usehivy/hivy/internal/spider"
	"github.com/usehivy/hivy/internal/webcrawl"
)

// buildWebProvider assembles the configured web crawl/search providers into a
// single webcrawl.Provider routed by the hardcoded per-operation hierarchy:
// scrape/crawl/map go spider then firecrawl; search goes serper then
// firecrawl (spider is deliberately not used for search). Providers are
// enabled by their API keys; returns nil when none is configured.
func buildWebProvider(cfg *config.Config) webcrawl.Provider {
	var spiderProvider, firecrawlProvider, serperProvider webcrawl.Provider
	if cfg.SpiderAPIKey != "" {
		spiderProvider = spider.NewProvider(spider.NewClient(cfg.SpiderAPIKey))
	}
	if cfg.FirecrawlAPIKey != "" {
		firecrawlProvider = firecrawl.NewProvider(firecrawl.NewClient(cfg.FirecrawlAPIKey))
	}
	if cfg.SerperAPIKey != "" {
		serperProvider = serper.NewProvider(serper.NewClient(cfg.SerperAPIKey))
	}

	scrape := configured(spiderProvider, firecrawlProvider)
	crawl := configured(spiderProvider, firecrawlProvider)
	search := configured(serperProvider, firecrawlProvider)
	siteMap := configured(spiderProvider, firecrawlProvider)

	if len(scrape) == 0 && len(crawl) == 0 && len(search) == 0 && len(siteMap) == 0 {
		return nil
	}
	return webcrawl.NewRouter(webcrawl.Hierarchy{
		Scrape: scrape,
		Crawl:  crawl,
		Search: search,
		Map:    siteMap,
		// Map backs the user-facing website discovery endpoint: each provider
		// gets 15s before the router falls back to the next one.
		MapTimeout: 15 * time.Second,
	})
}

func configured(providers ...webcrawl.Provider) []webcrawl.Provider {
	out := make([]webcrawl.Provider, 0, len(providers))
	for _, p := range providers {
		if p != nil {
			out = append(out, p)
		}
	}
	return out
}
