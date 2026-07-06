package website

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// defaultMaxPages caps a crawl (total across all seed URLs). Sites larger than
// this fall back to BFS-with-budget, which naturally prefers shallower (more
// central) pages. Sitemap discovery is enabled in the connector so curated URLs
// fill the budget first.
const defaultMaxPages = 500

type WebsiteConfig struct {
	// URL is the legacy single-seed field. Still accepted; folded into URLs.
	URL string `json:"url,omitempty"`
	// URLs is the set of seed URLs to ingest — section roots (crawled as a
	// subtree) and/or individual pages. Assembled from section discovery.
	URLs          []string `json:"urls,omitempty"`
	MaxPages      int      `json:"max_pages,omitempty"`
	RespectRobots *bool    `json:"respect_robots,omitempty"`
}

func LoadConfig(raw json.RawMessage) (WebsiteConfig, error) {
	cfg := WebsiteConfig{}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &cfg); err != nil {
			return WebsiteConfig{}, fmt.Errorf("website: parse config: %w", err)
		}
	}

	// Fold the legacy single url into the seed set, normalize, and de-duplicate.
	seeds := make([]string, 0, len(cfg.URLs)+1)
	if s := strings.TrimSpace(cfg.URL); s != "" {
		seeds = append(seeds, s)
	}
	seeds = append(seeds, cfg.URLs...)

	seen := map[string]struct{}{}
	normalized := make([]string, 0, len(seeds))
	for _, s := range seeds {
		s = strings.TrimSpace(s)
		if s == "" {
			continue
		}
		if !strings.Contains(s, "://") {
			s = "https://" + s
		}
		u, err := url.Parse(s)
		if err != nil || u.Host == "" {
			return WebsiteConfig{}, fmt.Errorf("website: invalid url %q", s)
		}
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		normalized = append(normalized, s)
	}
	if len(normalized) == 0 {
		return WebsiteConfig{}, fmt.Errorf("website: at least one url is required")
	}
	cfg.URLs = normalized
	cfg.URL = normalized[0] // keep URL pointing at the first seed for back-compat

	if cfg.MaxPages < 0 {
		return WebsiteConfig{}, fmt.Errorf("website: max_pages must be >= 0")
	}
	if cfg.MaxPages == 0 {
		cfg.MaxPages = defaultMaxPages
	}
	return cfg, nil
}

// seedWhitelist returns the Spider path whitelist for a seed URL so a section
// root (e.g. https://site/docs) only crawls its own subtree. A root seed
// ("/" or empty path) returns nil — crawl the whole site.
func seedWhitelist(seed string) []string {
	u, err := url.Parse(seed)
	if err != nil {
		return nil
	}
	path := strings.TrimRight(u.Path, "/")
	if path == "" {
		return nil
	}
	// Spider matches whitelist entries against the URL path; anchoring the
	// prefix keeps the crawl inside the section (e.g. "^/docs").
	return []string{"^" + path}
}
