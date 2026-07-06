package website

import (
	"context"
	"encoding/xml"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

// EstimateTotal reports how many pages the crawl is expected to index, so the
// UI can render a determinate progress bar. It reads the site's own sitemap
// (sitemap.xml, following one level of sitemap-index nesting) and counts the
// URLs, capped at MaxPages. When no usable sitemap is found it returns 0 — the
// caller then leaves docs_estimated unset and progress stays honestly a running
// count rather than showing a bar against a made-up denominator.
func (c *WebsiteConnector) EstimateTotal(ctx context.Context, src interfaces.Source) (int, error) {
	cfg, err := LoadConfig(src.Config())
	if err != nil {
		return 0, err
	}
	base, err := url.Parse(cfg.URLs[0])
	if err != nil || base.Host == "" {
		return 0, nil
	}

	// Derive path prefixes from the seed URLs. A root seed ("/" or empty path)
	// means "count the whole site"; otherwise count only sitemap URLs whose path
	// falls under one of the selected sections/pages.
	countAll := false
	var prefixes []string
	for _, seed := range cfg.URLs {
		u, err := url.Parse(seed)
		if err != nil {
			continue
		}
		p := strings.TrimRight(u.Path, "/")
		if p == "" {
			countAll = true
			break
		}
		prefixes = append(prefixes, p)
	}

	// Gather broadly when filtering by prefix (the sitemap holds the whole site);
	// when counting everything, stop at MaxPages.
	gatherCap := cfg.MaxPages
	if !countAll {
		gatherCap = 5000
	}
	sitemapURL := base.Scheme + "://" + base.Host + "/sitemap.xml"
	seen := make(map[string]struct{})
	countSitemapURLs(ctx, sitemapURL, seen, gatherCap, 0)

	if countAll {
		return min(len(seen), cfg.MaxPages), nil
	}
	matched := 0
	for u := range seen {
		if matchesAnyPrefix(u, prefixes) {
			matched++
		}
	}
	return min(matched, cfg.MaxPages), nil
}

// matchesAnyPrefix reports whether the URL's path starts with any of the given
// path prefixes.
func matchesAnyPrefix(rawURL string, prefixes []string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	path := u.Path
	for _, p := range prefixes {
		if path == p || strings.HasPrefix(path, p+"/") || strings.HasPrefix(path, p) {
			return true
		}
	}
	return false
}

type sitemapDoc struct {
	// A sitemap index lists child sitemaps under <sitemap><loc>; a urlset
	// lists pages under <url><loc>. We read both loc streams.
	SitemapLocs []string `xml:"sitemap>loc"`
	URLLocs     []string `xml:"url>loc"`
}

// countSitemapURLs fetches one sitemap and adds page URLs to seen. If the
// document is a sitemap index it recurses one level into each child sitemap.
// depth guards against pathological self-referential indexes.
func countSitemapURLs(ctx context.Context, sitemapURL string, seen map[string]struct{}, maxPages, depth int) {
	if depth > 1 || len(seen) >= maxPages {
		return
	}
	doc, ok := fetchSitemap(ctx, sitemapURL)
	if !ok {
		return
	}
	for _, loc := range doc.URLLocs {
		if len(seen) >= maxPages {
			return
		}
		if loc = strings.TrimSpace(loc); loc != "" {
			seen[loc] = struct{}{}
		}
	}
	for _, child := range doc.SitemapLocs {
		if len(seen) >= maxPages {
			return
		}
		if child = strings.TrimSpace(child); child != "" {
			countSitemapURLs(ctx, child, seen, maxPages, depth+1)
		}
	}
}

func fetchSitemap(ctx context.Context, sitemapURL string) (sitemapDoc, bool) {
	reqCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, sitemapURL, nil)
	if err != nil {
		return sitemapDoc{}, false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return sitemapDoc{}, false
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return sitemapDoc{}, false
	}
	// Cap the read at 10 MiB so a hostile/huge sitemap can't exhaust memory.
	body, err := io.ReadAll(io.LimitReader(resp.Body, 10<<20))
	if err != nil {
		return sitemapDoc{}, false
	}
	var doc sitemapDoc
	if err := xml.Unmarshal(body, &doc); err != nil {
		return sitemapDoc{}, false
	}
	return doc, true
}
