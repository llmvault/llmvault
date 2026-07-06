package website

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
)

// DiscoveredSection is a cluster of pages under a common top-level path (e.g.
// everything under /docs). Its URL is a crawl seed — ingesting the section
// crawls that subtree.
type DiscoveredSection struct {
	Label       string   `json:"label"`
	URL         string   `json:"url"`
	PathPrefix  string   `json:"path_prefix"`
	PageCount   int      `json:"page_count"`
	SamplePaths []string `json:"sample_paths"`
	// URLs is every page URL under this section (the full list, not the sample).
	URLs []string `json:"urls"`
}

// DiscoveredPage is a single page that didn't cluster into a section — the user
// can pick these individually.
type DiscoveredPage struct {
	URL  string `json:"url"`
	Path string `json:"path"`
}

// SectionDiscovery is what the discovery endpoint returns.
type SectionDiscovery struct {
	BaseURL  string              `json:"base_url"`
	Sections []DiscoveredSection `json:"sections"`
	Pages    []DiscoveredPage    `json:"pages"`
}

const (
	// defaultMinSectionSize: a top-level path needs at least this many pages to
	// be offered as a section rather than as individual pages.
	defaultMinSectionSize = 3
	maxSamplePaths        = 5
	maxUngroupedPages     = 100
)

// GroupLinks turns a flat list of discovered links into sections + individual
// pages, scoped to baseURL's host. A top-level path segment with >= minSize
// pages becomes a section; everything else is returned as individual pages.
func GroupLinks(baseURL string, links []string, minSize int) (SectionDiscovery, error) {
	if minSize <= 0 {
		minSize = defaultMinSectionSize
	}
	base, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || base.Host == "" {
		return SectionDiscovery{}, fmt.Errorf("website: invalid base url %q", baseURL)
	}
	origin := base.Scheme + "://" + base.Host

	// Group same-host paths by their first path segment, de-duplicated by path.
	type group struct {
		segment string
		paths   []string // sorted, unique
	}
	groups := map[string]*group{}
	seen := map[string]struct{}{}

	for _, raw := range links {
		u, err := url.Parse(strings.TrimSpace(raw))
		if err != nil || u.Host != base.Host {
			continue
		}
		path := normalizePath(u.Path)
		if _, dup := seen[path]; dup {
			continue
		}
		seen[path] = struct{}{}

		seg := firstSegment(path)
		g := groups[seg]
		if g == nil {
			g = &group{segment: seg}
			groups[seg] = g
		}
		g.paths = append(g.paths, path)
	}

	out := SectionDiscovery{BaseURL: origin}
	for _, g := range groups {
		sort.Strings(g.paths)
		// The home bucket (empty segment) and small buckets are individual pages.
		if g.segment == "" || len(g.paths) < minSize {
			for _, p := range g.paths {
				out.Pages = append(out.Pages, DiscoveredPage{URL: origin + p, Path: p})
			}
			continue
		}
		urls := make([]string, len(g.paths))
		for i, p := range g.paths {
			urls[i] = origin + p
		}
		out.Sections = append(out.Sections, DiscoveredSection{
			Label:       humanizeSegment(g.segment),
			URL:         origin + "/" + g.segment,
			PathPrefix:  "/" + g.segment,
			PageCount:   len(g.paths),
			SamplePaths: g.paths[:min(len(g.paths), maxSamplePaths)],
			URLs:        urls,
		})
	}

	// Largest sections first; pages alphabetical and capped.
	sort.Slice(out.Sections, func(i, j int) bool {
		if out.Sections[i].PageCount != out.Sections[j].PageCount {
			return out.Sections[i].PageCount > out.Sections[j].PageCount
		}
		return out.Sections[i].Label < out.Sections[j].Label
	})
	sort.Slice(out.Pages, func(i, j int) bool { return out.Pages[i].Path < out.Pages[j].Path })
	if len(out.Pages) > maxUngroupedPages {
		out.Pages = out.Pages[:maxUngroupedPages]
	}
	return out, nil
}

// normalizePath strips the fragment/query already dropped by the caller and
// removes a trailing slash so "/docs/" and "/docs" collapse. Root stays "/".
func normalizePath(p string) string {
	if p == "" {
		return "/"
	}
	if len(p) > 1 {
		p = strings.TrimRight(p, "/")
	}
	if p == "" {
		return "/"
	}
	return p
}

// firstSegment returns the first path segment ("" for the site root).
func firstSegment(path string) string {
	trimmed := strings.Trim(path, "/")
	if trimmed == "" {
		return ""
	}
	if i := strings.IndexByte(trimmed, '/'); i >= 0 {
		return trimmed[:i]
	}
	return trimmed
}

// humanizeSegment turns "getting-started" / "api_docs" into "Getting Started".
func humanizeSegment(seg string) string {
	seg = strings.ReplaceAll(seg, "-", " ")
	seg = strings.ReplaceAll(seg, "_", " ")
	words := strings.Fields(seg)
	for i, w := range words {
		if w == "" {
			continue
		}
		words[i] = strings.ToUpper(w[:1]) + w[1:]
	}
	return strings.Join(words, " ")
}
