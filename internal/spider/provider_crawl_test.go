package spider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/webcrawl"
)

func TestProviderCrawl_SkipsErrorPages(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{
		{URL: "https://example.com", Content: "one", StatusCode: 200},
		{URL: "https://example.com/bad", Error: "timeout", StatusCode: 200},
		{URL: "https://example.com/two", Content: "two", StatusCode: 200},
	}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	depth := 3
	pages, err := provider.Crawl(context.Background(), webcrawl.CrawlRequest{
		URL:   "https://example.com",
		Limit: 5,
		Depth: depth,
	})
	if err != nil {
		t.Fatalf("Crawl() error: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 usable pages, got %d", len(pages))
	}
	if pages[0].Content != "one" || pages[1].Content != "two" {
		t.Errorf("unexpected pages: %+v", pages)
	}

	mu.Lock()
	defer mu.Unlock()
	sent := decodeBody(t, captured.Body)
	if sent["limit"] != float64(5) {
		t.Errorf("expected limit 5, got %v", sent["limit"])
	}
	if sent["depth"] != float64(3) {
		t.Errorf("expected depth 3, got %v", sent["depth"])
	}
}

func TestProviderCrawl_AllErrorPages(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{
		{URL: "https://example.com", Error: "boom", StatusCode: 200},
		{URL: "https://example.com/two", Error: "boom", StatusCode: 200},
	}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	_, err := provider.Crawl(context.Background(), webcrawl.CrawlRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error when all pages fail, got nil")
	}
}

func TestProviderSearch_Mapping(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := SearchResponse{Content: []SearchResult{
		{Title: "One", URL: "https://a.example", Description: "first"},
		{Title: "Two", URL: "https://b.example", Description: "second"},
	}}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	results, err := provider.Search(context.Background(), webcrawl.SearchRequest{Query: "hello", Limit: 4})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "One" || results[0].URL != "https://a.example" || results[0].Description != "first" {
		t.Errorf("unexpected first result: %+v", results[0])
	}

	mu.Lock()
	defer mu.Unlock()
	if captured.Path != "/v1/search" {
		t.Errorf("expected path /v1/search, got %s", captured.Path)
	}
	sent := decodeBody(t, captured.Body)
	if sent["search"] != "hello" {
		t.Errorf("expected search hello, got %v", sent["search"])
	}
	if sent["num"] != float64(4) {
		t.Errorf("expected num 4, got %v", sent["num"])
	}
	if sent["search_limit"] != float64(4) {
		t.Errorf("expected search_limit 4, got %v", sent["search_limit"])
	}
	if sent["request"] != "smart" {
		t.Errorf("expected request smart, got %v", sent["request"])
	}
}

func TestProviderSearch_BareArrayResponse(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"url":"https://a.example","title":"One","description":"first"}]`))
	}))
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	results, err := provider.Search(context.Background(), webcrawl.SearchRequest{Query: "hello"})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://a.example" || results[0].Title != "One" {
		t.Fatalf("unexpected results: %+v", results)
	}
}

func TestProviderSearch_EmptyIsNotError(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := SearchResponse{Content: []SearchResult{}}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	results, err := provider.Search(context.Background(), webcrawl.SearchRequest{Query: "nothing"})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(results))
	}
}

func TestProviderMap_AggregatesAndDedups(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{
		{URL: "https://example.com", Links: []string{"https://example.com/a", "https://example.com/b"}},
		{URL: "https://example.com/a", Links: []string{"https://example.com/b", "https://example.com/c"}},
	}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	urls, err := provider.Map(context.Background(), webcrawl.MapRequest{URL: "https://example.com", Limit: 20})
	if err != nil {
		t.Fatalf("Map() error: %v", err)
	}

	want := []string{
		"https://example.com",
		"https://example.com/a",
		"https://example.com/b",
		"https://example.com/c",
	}
	if len(urls) != len(want) {
		t.Fatalf("expected %d urls, got %d: %v", len(want), len(urls), urls)
	}
	for i := range want {
		if urls[i] != want[i] {
			t.Errorf("url[%d] = %q, want %q", i, urls[i], want[i])
		}
	}

	mu.Lock()
	defer mu.Unlock()
	if captured.Path != "/v1/links" {
		t.Errorf("expected path /v1/links, got %s", captured.Path)
	}
	sent := decodeBody(t, captured.Body)
	if sent["return_page_links"] != true {
		t.Errorf("expected return_page_links true, got %v", sent["return_page_links"])
	}
	if sent["respect_robots"] != true {
		t.Errorf("expected respect_robots true, got %v", sent["respect_robots"])
	}
	if sent["limit"] != float64(20) {
		t.Errorf("expected limit 20, got %v", sent["limit"])
	}
}

func TestProviderMap_ZeroURLsIsError(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{{URL: ""}}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	_, err := provider.Map(context.Background(), webcrawl.MapRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for zero URLs, got nil")
	}
}
