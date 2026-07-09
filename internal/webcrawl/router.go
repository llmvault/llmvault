package webcrawl

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
)

// Hierarchy is the per-operation provider priority: earlier providers are
// preferred, later ones are fallbacks tried in order when a call fails.
// The *Timeout fields bound each provider's attempt at that operation — a
// provider exceeding it is cut off and the next one gets its own fresh
// budget. Zero means no per-attempt bound (the caller's context still
// applies).
type Hierarchy struct {
	Scrape []Provider
	Crawl  []Provider
	Search []Provider
	Map    []Provider

	ScrapeTimeout time.Duration
	CrawlTimeout  time.Duration
	SearchTimeout time.Duration
	MapTimeout    time.Duration
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
	return fallback(ctx, r.hierarchy.Scrape, "scrape", r.hierarchy.ScrapeTimeout, func(ctx context.Context, p Provider) (Page, error) {
		return p.Scrape(ctx, req)
	})
}

func (r *Router) Crawl(ctx context.Context, req CrawlRequest) ([]Page, error) {
	return fallback(ctx, r.hierarchy.Crawl, "crawl", r.hierarchy.CrawlTimeout, func(ctx context.Context, p Provider) ([]Page, error) {
		return p.Crawl(ctx, req)
	})
}

func (r *Router) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	return fallback(ctx, r.hierarchy.Search, "search", r.hierarchy.SearchTimeout, func(ctx context.Context, p Provider) ([]SearchResult, error) {
		return p.Search(ctx, req)
	})
}

func (r *Router) Map(ctx context.Context, req MapRequest) ([]string, error) {
	return fallback(ctx, r.hierarchy.Map, "map", r.hierarchy.MapTimeout, func(ctx context.Context, p Provider) ([]string, error) {
		return p.Map(ctx, req)
	})
}

func fallback[T any](ctx context.Context, providers []Provider, operation string, attemptTimeout time.Duration, call func(context.Context, Provider) (T, error)) (T, error) {
	var zero T
	var errs []error
	for _, p := range providers {
		result, err := attempt(ctx, attemptTimeout, p, call)
		if err != nil {
			logging.FromContext(ctx).WarnContext(ctx, "webcrawl provider failed",
				"provider", p.Name(), "operation", operation, "error", err)
			errs = append(errs, err)
			if ctx.Err() != nil {
				break
			}
			continue
		}
		return result, nil
	}
	if len(errs) == 0 {
		return zero, fmt.Errorf("webcrawl: no provider configured for %s", operation)
	}
	return zero, errors.Join(errs...)
}

func attempt[T any](ctx context.Context, timeout time.Duration, p Provider, call func(context.Context, Provider) (T, error)) (T, error) {
	if timeout <= 0 {
		return call(ctx, p)
	}
	attemptCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return call(attemptCtx, p)
}

var _ Provider = (*Router)(nil)
