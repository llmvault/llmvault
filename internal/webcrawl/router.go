package webcrawl

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/usehivy/hivy/internal/logging"
)

// Hierarchy is the per-operation provider priority: earlier providers are
// preferred, later ones are fallbacks tried in order when a call fails.
type Hierarchy struct {
	Scrape []Provider
	Crawl  []Provider
	Search []Provider
	Map    []Provider
}

// Router dispatches each operation across its Hierarchy, returning the first
// success and falling through to the next provider on error. If every
// provider fails, the joined error is returned.
type Router struct {
	hierarchy Hierarchy
}

// NewRouter builds a Router over the given per-operation hierarchy.
func NewRouter(hierarchy Hierarchy) *Router {
	return &Router{hierarchy: hierarchy}
}

// Name reports the router's identity as "router(name1,name2,...)", listing
// distinct providers across all operations in first-seen order.
func (r *Router) Name() string {
	seen := map[string]struct{}{}
	names := make([]string, 0, 2)
	for _, providers := range [][]Provider{r.hierarchy.Scrape, r.hierarchy.Crawl, r.hierarchy.Search, r.hierarchy.Map} {
		for _, p := range providers {
			if _, ok := seen[p.Name()]; ok {
				continue
			}
			seen[p.Name()] = struct{}{}
			names = append(names, p.Name())
		}
	}
	return "router(" + strings.Join(names, ",") + ")"
}

func (r *Router) Scrape(ctx context.Context, req ScrapeRequest) (Page, error) {
	return fallback(ctx, r.hierarchy.Scrape, "scrape", func(p Provider) (Page, error) {
		return p.Scrape(ctx, req)
	})
}

func (r *Router) Crawl(ctx context.Context, req CrawlRequest) ([]Page, error) {
	return fallback(ctx, r.hierarchy.Crawl, "crawl", func(p Provider) ([]Page, error) {
		return p.Crawl(ctx, req)
	})
}

func (r *Router) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	return fallback(ctx, r.hierarchy.Search, "search", func(p Provider) ([]SearchResult, error) {
		return p.Search(ctx, req)
	})
}

func (r *Router) Map(ctx context.Context, req MapRequest) ([]string, error) {
	return fallback(ctx, r.hierarchy.Map, "map", func(p Provider) ([]string, error) {
		return p.Map(ctx, req)
	})
}

func fallback[T any](ctx context.Context, providers []Provider, operation string, call func(Provider) (T, error)) (T, error) {
	var zero T
	var errs []error
	for _, p := range providers {
		result, err := call(p)
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "webcrawl provider failed",
				"provider", p.Name(), "operation", operation, "error", err)
			errs = append(errs, err)
			continue
		}
		return result, nil
	}
	if len(errs) == 0 {
		return zero, fmt.Errorf("webcrawl: no provider configured for %s", operation)
	}
	return zero, errors.Join(errs...)
}

var _ Provider = (*Router)(nil)
