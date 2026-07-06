package website

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"
)

// defaultMaxPages caps how many URLs a single source will scrape, as a
// safety limit on a pathologically large URL list.
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
