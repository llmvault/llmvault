package webcrawl

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeProvider struct {
	name string

	scrape func(context.Context, ScrapeRequest) (Page, error)
	crawl  func(context.Context, CrawlRequest) ([]Page, error)
	search func(context.Context, SearchRequest) ([]SearchResult, error)
	mapFn  func(context.Context, MapRequest) ([]string, error)

	calls int
}

func (f *fakeProvider) Name() string { return f.name }

func (f *fakeProvider) Scrape(ctx context.Context, req ScrapeRequest) (Page, error) {
	f.calls++
	return f.scrape(ctx, req)
}

func (f *fakeProvider) Crawl(ctx context.Context, req CrawlRequest) ([]Page, error) {
	f.calls++
	return f.crawl(ctx, req)
}

func (f *fakeProvider) Search(ctx context.Context, req SearchRequest) ([]SearchResult, error) {
	f.calls++
	return f.search(ctx, req)
}

func (f *fakeProvider) Map(ctx context.Context, req MapRequest) ([]string, error) {
	f.calls++
	return f.mapFn(ctx, req)
}

func okScrape(page Page) func(context.Context, ScrapeRequest) (Page, error) {
	return func(context.Context, ScrapeRequest) (Page, error) { return page, nil }
}

func errScrape(err error) func(context.Context, ScrapeRequest) (Page, error) {
	return func(context.Context, ScrapeRequest) (Page, error) { return Page{}, err }
}

func uniformHierarchy(providers ...Provider) Hierarchy {
	return Hierarchy{Scrape: providers, Crawl: providers, Search: providers, Map: providers}
}

func TestRouterName(t *testing.T) {
	r := NewRouter(uniformHierarchy(&fakeProvider{name: "spider"}, &fakeProvider{name: "firecrawl"}))
	if got, want := r.Name(), "router(spider,firecrawl)"; got != want {
		t.Fatalf("Name() = %q, want %q", got, want)
	}
}

func TestRouterScrape(t *testing.T) {
	errFirst := errors.New("first boom")
	errSecond := errors.New("second boom")

	tests := []struct {
		name        string
		first       *fakeProvider
		second      *fakeProvider
		wantContent string
		wantErr     bool
		wantErrHas  []string
		firstCalls  int
		secondCalls int
	}{
		{
			name:        "success on first, second never called",
			first:       &fakeProvider{name: "a", scrape: okScrape(Page{URL: "u", Content: "one"})},
			second:      &fakeProvider{name: "b", scrape: okScrape(Page{URL: "u", Content: "two"})},
			wantContent: "one",
			firstCalls:  1,
			secondCalls: 0,
		},
		{
			name:        "fallback on error",
			first:       &fakeProvider{name: "a", scrape: errScrape(errFirst)},
			second:      &fakeProvider{name: "b", scrape: okScrape(Page{URL: "u", Content: "two"})},
			wantContent: "two",
			firstCalls:  1,
			secondCalls: 1,
		},
		{
			name:        "all fail returns joined error",
			first:       &fakeProvider{name: "a", scrape: errScrape(errFirst)},
			second:      &fakeProvider{name: "b", scrape: errScrape(errSecond)},
			wantErr:     true,
			wantErrHas:  []string{"first boom", "second boom"},
			firstCalls:  1,
			secondCalls: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter(uniformHierarchy(tt.first, tt.second))
			page, err := r.Scrape(context.Background(), ScrapeRequest{URL: "u"})
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				for _, sub := range tt.wantErrHas {
					if !strings.Contains(err.Error(), sub) {
						t.Errorf("error %q missing %q", err.Error(), sub)
					}
				}
			} else {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if page.Content != tt.wantContent {
					t.Errorf("content = %q, want %q", page.Content, tt.wantContent)
				}
			}
			if tt.first.calls != tt.firstCalls {
				t.Errorf("first provider calls = %d, want %d", tt.first.calls, tt.firstCalls)
			}
			if tt.second.calls != tt.secondCalls {
				t.Errorf("second provider calls = %d, want %d", tt.second.calls, tt.secondCalls)
			}
		})
	}
}

func TestRouterCrawl(t *testing.T) {
	errFirst := errors.New("crawl-a-fail")
	first := &fakeProvider{name: "a", crawl: func(context.Context, CrawlRequest) ([]Page, error) {
		return nil, errFirst
	}}
	second := &fakeProvider{name: "b", crawl: func(context.Context, CrawlRequest) ([]Page, error) {
		return []Page{{URL: "u", Content: "c"}}, nil
	}}
	r := NewRouter(uniformHierarchy(first, second))
	pages, err := r.Crawl(context.Background(), CrawlRequest{URL: "u"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(pages) != 1 || pages[0].Content != "c" {
		t.Fatalf("unexpected pages: %+v", pages)
	}
	if first.calls != 1 || second.calls != 1 {
		t.Fatalf("call counts: first=%d second=%d", first.calls, second.calls)
	}
}

func TestRouterSearch(t *testing.T) {
	first := &fakeProvider{name: "a", search: func(context.Context, SearchRequest) ([]SearchResult, error) {
		return nil, errors.New("search-a-fail")
	}}
	second := &fakeProvider{name: "b", search: func(context.Context, SearchRequest) ([]SearchResult, error) {
		return []SearchResult{{URL: "u", Title: "t"}}, nil
	}}
	r := NewRouter(uniformHierarchy(first, second))
	results, err := r.Search(context.Background(), SearchRequest{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Title != "t" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestRouterMap(t *testing.T) {
	errFirst := errors.New("map-a-fail")
	errSecond := errors.New("map-b-fail")
	first := &fakeProvider{name: "a", mapFn: func(context.Context, MapRequest) ([]string, error) {
		return nil, errFirst
	}}
	second := &fakeProvider{name: "b", mapFn: func(context.Context, MapRequest) ([]string, error) {
		return nil, errSecond
	}}
	r := NewRouter(uniformHierarchy(first, second))
	urls, err := r.Map(context.Background(), MapRequest{URL: "u"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if urls != nil {
		t.Fatalf("expected nil urls, got %+v", urls)
	}
	for _, sub := range []string{"map-a-fail", "map-b-fail"} {
		if !strings.Contains(err.Error(), sub) {
			t.Errorf("error %q missing %q", err.Error(), sub)
		}
	}
}

func TestRouterMapAttemptTimeoutFallsBack(t *testing.T) {
	hung := &fakeProvider{name: "hung", mapFn: func(ctx context.Context, _ MapRequest) ([]string, error) {
		<-ctx.Done()
		return nil, ctx.Err()
	}}
	fast := &fakeProvider{name: "fast", mapFn: func(context.Context, MapRequest) ([]string, error) {
		return []string{"https://example.com"}, nil
	}}
	r := NewRouter(Hierarchy{Map: []Provider{hung, fast}, MapTimeout: 20 * time.Millisecond})

	start := time.Now()
	urls, err := r.Map(context.Background(), MapRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 1 || urls[0] != "https://example.com" {
		t.Fatalf("unexpected urls: %+v", urls)
	}
	if hung.calls != 1 || fast.calls != 1 {
		t.Fatalf("call counts: hung=%d fast=%d", hung.calls, fast.calls)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("fallback took %s; per-attempt timeout not applied", elapsed)
	}
}

func TestRouterNoAttemptTimeoutPassesParentContext(t *testing.T) {
	var hadDeadline bool
	p := &fakeProvider{name: "a", mapFn: func(ctx context.Context, _ MapRequest) ([]string, error) {
		_, hadDeadline = ctx.Deadline()
		return []string{"u"}, nil
	}}
	r := NewRouter(Hierarchy{Map: []Provider{p}})
	if _, err := r.Map(context.Background(), MapRequest{URL: "u"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if hadDeadline {
		t.Fatal("provider context has a deadline; zero timeout must not add one")
	}
}

func TestRouterStopsWhenParentContextDone(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	first := &fakeProvider{name: "a", mapFn: func(context.Context, MapRequest) ([]string, error) {
		cancel()
		return nil, errors.New("boom")
	}}
	second := &fakeProvider{name: "b", mapFn: func(context.Context, MapRequest) ([]string, error) {
		return []string{"u"}, nil
	}}
	r := NewRouter(Hierarchy{Map: []Provider{first, second}})
	if _, err := r.Map(ctx, MapRequest{URL: "u"}); err == nil {
		t.Fatal("expected error, got nil")
	}
	if second.calls != 0 {
		t.Fatalf("second provider called %d times after parent context ended", second.calls)
	}
}

func TestRouterPerOperationHierarchy(t *testing.T) {
	scrapeOnly := &fakeProvider{name: "a", scrape: okScrape(Page{URL: "u", Content: "one"})}
	searchOnly := &fakeProvider{name: "b", search: func(context.Context, SearchRequest) ([]SearchResult, error) {
		return []SearchResult{{URL: "u"}}, nil
	}}
	r := NewRouter(Hierarchy{Scrape: []Provider{scrapeOnly}, Search: []Provider{searchOnly}})

	if _, err := r.Scrape(context.Background(), ScrapeRequest{URL: "u"}); err != nil {
		t.Fatalf("scrape: unexpected error: %v", err)
	}
	if _, err := r.Search(context.Background(), SearchRequest{Query: "q"}); err != nil {
		t.Fatalf("search: unexpected error: %v", err)
	}
	if _, err := r.Crawl(context.Background(), CrawlRequest{URL: "u"}); err == nil {
		t.Fatal("crawl with empty hierarchy: expected error, got nil")
	}
	if scrapeOnly.calls != 1 || searchOnly.calls != 1 {
		t.Fatalf("call counts: scrapeOnly=%d searchOnly=%d", scrapeOnly.calls, searchOnly.calls)
	}
}
