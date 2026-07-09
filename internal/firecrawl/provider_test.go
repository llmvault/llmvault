package firecrawl

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/webcrawl"
)

func newProvider(srvURL string) *Provider {
	client := newClientWithBaseURL(srvURL, "test-key")
	client.pollInterval = time.Millisecond
	return NewProvider(client)
}

func TestProviderName(t *testing.T) {
	if NewProvider(newClientWithBaseURL("", "")).Name() != "firecrawl" {
		t.Fatal("Name() must be firecrawl")
	}
}

func TestProvider_ScrapePageMapping(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"markdown":"# Body","html":null,"rawHtml":null,"links":[],"metadata":{"sourceURL":"https://src.example.com","url":"https://final.example.com","statusCode":200,"error":null},"warning":null}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	page, err := newProvider(srv.URL).Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://req.example.com"})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if page.URL != "https://src.example.com" || page.Content != "# Body" || page.StatusCode != 200 {
		t.Fatalf("page = %+v", page)
	}

	mu.Lock()
	defer mu.Unlock()
	var sent map[string]any
	if err := json.Unmarshal([]byte(reqs[0].Body), &sent); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	formats := sent["formats"].([]any)
	if len(formats) != 1 || formats[0] != "markdown" {
		t.Fatalf("formats = %v", formats)
	}
	if sent["onlyMainContent"] != true {
		t.Fatalf("onlyMainContent = %v", sent["onlyMainContent"])
	}
}

func TestProvider_ScrapeSourceURLFallback(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"markdown":"# Body","metadata":{"sourceURL":"","url":"https://final.example.com","statusCode":200,"error":null}}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	page, err := newProvider(srv.URL).Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://req.example.com"})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if page.URL != "https://final.example.com" {
		t.Fatalf("expected url fallback, got %q", page.URL)
	}
}

func TestProvider_ScrapeRequestURLFallback(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"markdown":"# Body","metadata":{"sourceURL":"","url":"","statusCode":200,"error":null}}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	page, err := newProvider(srv.URL).Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://req.example.com"})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if page.URL != "https://req.example.com" {
		t.Fatalf("expected request-url fallback, got %q", page.URL)
	}
}

func TestProvider_ScrapeMetadataErrorFails(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"markdown":"","metadata":{"sourceURL":"https://example.com","url":"","statusCode":403,"error":"forbidden"}}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	_, err := newProvider(srv.URL).Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "forbidden") {
		t.Fatalf("expected metadata error, got %v", err)
	}
}

func TestProvider_ScrapeEmptyContentFails(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"markdown":"","metadata":{"sourceURL":"https://example.com","url":"","statusCode":200,"error":null}}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	_, err := newProvider(srv.URL).Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "empty content") {
		t.Fatalf("expected empty content error, got %v", err)
	}
}

func TestProvider_ScrapeHTMLFormat(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"markdown":"","html":"<h1>Hi</h1>","metadata":{"sourceURL":"https://example.com","url":"","statusCode":200,"error":null}}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	page, err := newProvider(srv.URL).Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://example.com", Format: webcrawl.FormatHTML})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if page.Content != "<h1>Hi</h1>" {
		t.Fatalf("content = %q", page.Content)
	}

	mu.Lock()
	defer mu.Unlock()
	var sent map[string]any
	_ = json.Unmarshal([]byte(reqs[0].Body), &sent)
	formats := sent["formats"].([]any)
	if len(formats) != 1 || formats[0] != "html" {
		t.Fatalf("formats = %v", formats)
	}
}

func TestProvider_ScrapeTextMapsToMarkdown(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"markdown":"# Text","metadata":{"sourceURL":"https://example.com","url":"","statusCode":200,"error":null}}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	page, err := newProvider(srv.URL).Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://example.com", Format: webcrawl.FormatText})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if page.Content != "# Text" {
		t.Fatalf("content = %q", page.Content)
	}

	mu.Lock()
	defer mu.Unlock()
	var sent map[string]any
	_ = json.Unmarshal([]byte(reqs[0].Body), &sent)
	formats := sent["formats"].([]any)
	if formats[0] != "markdown" {
		t.Fatalf("text should map to markdown, got %v", formats)
	}
}

func TestProvider_CrawlFullFlow(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const startRaw = `{"success":true,"id":"job-1","url":"https://api.firecrawl.dev/v2/crawl/job-1"}`
	const scrapingRaw = `{"status":"scraping","total":2,"completed":0,"creditsUsed":0,"expiresAt":"","next":null,"data":[]}`
	const completedRaw = `{"status":"completed","total":2,"completed":2,"creditsUsed":10,"expiresAt":"","next":null,"data":[{"markdown":"# Page A","metadata":{"sourceURL":"https://example.com/a","url":"","statusCode":200,"error":null}},{"markdown":"# Page B","metadata":{"sourceURL":"https://example.com/b","url":"","statusCode":200,"error":null}}]}`

	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeJSON(w, http.StatusOK, startRaw)
			return
		}
		mu.Lock()
		gets := 0
		for _, rq := range reqs {
			if rq.Method == http.MethodGet {
				gets++
			}
		}
		mu.Unlock()
		if gets == 1 {
			writeJSON(w, http.StatusOK, scrapingRaw)
			return
		}
		writeJSON(w, http.StatusOK, completedRaw)
	})
	t.Cleanup(srv.Close)

	pages, err := newProvider(srv.URL).Crawl(context.Background(), webcrawl.CrawlRequest{URL: "https://example.com", Limit: 5, Depth: 2})
	if err != nil {
		t.Fatalf("Crawl() error: %v", err)
	}
	if len(pages) != 2 || pages[0].URL != "https://example.com/a" || pages[1].Content != "# Page B" {
		t.Fatalf("pages = %+v", pages)
	}

	mu.Lock()
	defer mu.Unlock()
	var sent map[string]any
	if err := json.Unmarshal([]byte(reqs[0].Body), &sent); err != nil {
		t.Fatalf("unmarshal start body: %v", err)
	}
	if sent["limit"].(float64) != 5 || sent["maxDiscoveryDepth"].(float64) != 2 {
		t.Fatalf("limit/depth = %v %v", sent["limit"], sent["maxDiscoveryDepth"])
	}
	opts := sent["scrapeOptions"].(map[string]any)
	formats := opts["formats"].([]any)
	if formats[0] != "markdown" {
		t.Fatalf("scrapeOptions.formats = %v", formats)
	}
}

func TestProvider_CrawlPagination(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	var srvURL string
	const startRaw = `{"success":true,"id":"job-2","url":"x"}`

	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeJSON(w, http.StatusOK, startRaw)
			return
		}
		if r.URL.RawQuery == "offset=1" {
			writeJSON(w, http.StatusOK, `{"status":"completed","next":null,"data":[{"markdown":"# Page B","metadata":{"sourceURL":"https://example.com/b","url":"","statusCode":200,"error":null}}]}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"status":"completed","next":"`+srvURL+`/v2/crawl/job-2?offset=1","data":[{"markdown":"# Page A","metadata":{"sourceURL":"https://example.com/a","url":"","statusCode":200,"error":null}}]}`)
	})
	t.Cleanup(srv.Close)
	srvURL = srv.URL

	pages, err := newProvider(srv.URL).Crawl(context.Background(), webcrawl.CrawlRequest{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Crawl() error: %v", err)
	}
	if len(pages) != 2 || pages[0].URL != "https://example.com/a" || pages[1].URL != "https://example.com/b" {
		t.Fatalf("pages = %+v", pages)
	}
}

func TestProvider_CrawlFailedStatus(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeJSON(w, http.StatusOK, `{"success":true,"id":"job-3","url":"x"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"status":"failed","next":null,"data":[]}`)
	})
	t.Cleanup(srv.Close)

	_, err := newProvider(srv.URL).Crawl(context.Background(), webcrawl.CrawlRequest{URL: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "failed") {
		t.Fatalf("expected failed error, got %v", err)
	}
}

func TestProvider_CrawlCancelledStatus(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeJSON(w, http.StatusOK, `{"success":true,"id":"job-5","url":"x"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"status":"cancelled","next":null,"data":[]}`)
	})
	t.Cleanup(srv.Close)

	_, err := newProvider(srv.URL).Crawl(context.Background(), webcrawl.CrawlRequest{URL: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "cancelled") {
		t.Fatalf("expected cancelled error, got %v", err)
	}
}

func TestProvider_CrawlZeroUsableFails(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost {
			writeJSON(w, http.StatusOK, `{"success":true,"id":"job-4","url":"x"}`)
			return
		}
		writeJSON(w, http.StatusOK, `{"status":"completed","next":null,"data":[{"markdown":"","metadata":{"sourceURL":"https://example.com/a","url":"","statusCode":200,"error":"blocked"}}]}`)
	})
	t.Cleanup(srv.Close)

	_, err := newProvider(srv.URL).Crawl(context.Background(), webcrawl.CrawlRequest{URL: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "no usable pages") {
		t.Fatalf("expected no usable pages error, got %v", err)
	}
}

func TestProvider_SearchMapping(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"web":[{"title":"One","description":"first","url":"https://a.com"}]},"creditsUsed":1}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	results, err := newProvider(srv.URL).Search(context.Background(), webcrawl.SearchRequest{Query: "q", Limit: 3})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 1 || results[0].URL != "https://a.com" || results[0].Title != "One" {
		t.Fatalf("results = %+v", results)
	}
}

func TestProvider_SearchEmptyIsNotError(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"web":[]},"creditsUsed":1}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	results, err := newProvider(srv.URL).Search(context.Background(), webcrawl.SearchRequest{Query: "q"})
	if err != nil {
		t.Fatalf("empty search must not error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty, got %d", len(results))
	}
}

func TestProvider_MapDedup(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"links":[{"url":"https://example.com/a","title":"A"},{"url":"https://example.com/a","title":"A dup"},{"url":"https://example.com/b","title":"B"}]}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	links, err := newProvider(srv.URL).Map(context.Background(), webcrawl.MapRequest{URL: "https://example.com", Limit: 10})
	if err != nil {
		t.Fatalf("Map() error: %v", err)
	}
	if len(links) != 2 || links[0] != "https://example.com/a" || links[1] != "https://example.com/b" {
		t.Fatalf("links = %+v", links)
	}

	mu.Lock()
	defer mu.Unlock()
	var sent map[string]any
	_ = json.Unmarshal([]byte(reqs[0].Body), &sent)
	if sent["limit"].(float64) != 10 {
		t.Fatalf("limit = %v", sent["limit"])
	}
	if sent["includeSubdomains"] != false {
		t.Fatalf("includeSubdomains = %v, want false", sent["includeSubdomains"])
	}
}

func TestProvider_MapZeroIsError(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"links":[]}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	_, err := newProvider(srv.URL).Map(context.Background(), webcrawl.MapRequest{URL: "https://example.com"})
	if err == nil || !strings.Contains(err.Error(), "no links") {
		t.Fatalf("expected no links error, got %v", err)
	}
}
