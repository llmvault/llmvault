package spider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/usehivy/hivy/internal/webcrawl"
)

func TestProviderScrape_Success(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{{Content: "# Hello", URL: "https://example.com", StatusCode: 200}}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	page, err := provider.Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if page.URL != "https://example.com" || page.Content != "# Hello" || page.StatusCode != 200 {
		t.Fatalf("unexpected page: %+v", page)
	}

	mu.Lock()
	defer mu.Unlock()
	if captured.Path != "/v1/crawl" {
		t.Errorf("expected path /v1/crawl, got %s", captured.Path)
	}
	sent := decodeBody(t, captured.Body)
	if sent["return_format"] != "markdown" {
		t.Errorf("expected return_format markdown, got %v", sent["return_format"])
	}
	if sent["readability"] != true {
		t.Errorf("expected readability true, got %v", sent["readability"])
	}
	if sent["limit"] != float64(1) {
		t.Errorf("expected limit 1, got %v", sent["limit"])
	}
	if sent["request"] != "smart" {
		t.Errorf("expected request smart, got %v", sent["request"])
	}
}

func TestProviderScrape_URLFallback(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{{Content: "body", StatusCode: 200}}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	page, err := provider.Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://fallback.example"})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if page.URL != "https://fallback.example" {
		t.Errorf("expected fallback url, got %q", page.URL)
	}
}

func TestProviderScrape_PageError(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{{URL: "https://example.com", Error: "blocked", StatusCode: 200}}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	_, err := provider.Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for page error, got nil")
	}
}

func TestProviderScrape_NonSuccessStatus(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{{URL: "https://example.com", Content: "not found", StatusCode: 404}}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	_, err := provider.Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for 404 status, got nil")
	}
}

func TestProviderScrape_HTMLFormatMapsToRaw(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{{Content: "<h1>Hi</h1>", URL: "https://example.com", StatusCode: 200}}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	_, err := provider.Scrape(context.Background(), webcrawl.ScrapeRequest{
		URL:    "https://example.com",
		Format: webcrawl.FormatHTML,
	})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	sent := decodeBody(t, captured.Body)
	if sent["return_format"] != "raw" {
		t.Errorf("expected return_format raw for html, got %v", sent["return_format"])
	}
	if _, ok := sent["readability"]; ok {
		t.Errorf("expected readability omitted for raw html, got %v", sent["readability"])
	}
}

func TestProviderScrape_TextFormat(t *testing.T) {
	var captured capturedRequest
	var mu sync.Mutex

	resp := []Response{{Content: "plain", URL: "https://example.com", StatusCode: 200}}
	srv := mockSpiderAPI(t, &captured, &mu, http.StatusOK, resp)
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	_, err := provider.Scrape(context.Background(), webcrawl.ScrapeRequest{
		URL:    "https://example.com",
		Format: webcrawl.FormatText,
	})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	sent := decodeBody(t, captured.Body)
	if sent["return_format"] != "text" {
		t.Errorf("expected return_format text, got %v", sent["return_format"])
	}
}

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

func TestProviderScrape_StatusFieldName(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"url":"https://example.com","content":"ok","status":200}]`))
	}))
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	page, err := provider.Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if page.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200 (decoded from \"status\")", page.StatusCode)
	}
}

func TestProviderScrape_StatusFieldNameNonSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`[{"url":"https://example.com","content":"not found","status":404}]`))
	}))
	t.Cleanup(srv.Close)

	provider := NewProvider(newClientWithEndpoint(srv.URL, "test-key"))
	_, err := provider.Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error for status 404, got nil")
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

func decodeBody(t *testing.T, body string) map[string]any {
	t.Helper()
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	return sent
}
