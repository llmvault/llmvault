package firecrawl

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"
)

type capturedRequest struct {
	Method string
	Path   string
	Query  string
	Auth   string
	Accept string
	CType  string
	Body   string
}

func newCapture(t *testing.T, mu *sync.Mutex, captured *[]capturedRequest, handler http.HandlerFunc) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		mu.Lock()
		*captured = append(*captured, capturedRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Query:  r.URL.RawQuery,
			Auth:   r.Header.Get("Authorization"),
			Accept: r.Header.Get("Accept"),
			CType:  r.Header.Get("Content-Type"),
			Body:   string(body),
		})
		mu.Unlock()
		handler(w, r)
	}))
}

func writeJSON(w http.ResponseWriter, status int, raw string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = io.WriteString(w, raw)
}

func TestScrape_HappyPath(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"markdown":"# Hello","html":null,"rawHtml":null,"links":["https://example.com/a"],"metadata":{"sourceURL":"https://example.com","url":"https://example.com/final","statusCode":200,"error":null},"warning":null}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	client := newClientWithBaseURL(srv.URL, "test-key")
	data, err := client.Scrape(context.Background(), ScrapeParams{URL: "https://example.com", Formats: []string{"markdown"}, OnlyMainContent: true})
	if err != nil {
		t.Fatalf("Scrape() error: %v", err)
	}
	if data.Markdown != "# Hello" {
		t.Fatalf("markdown = %q", data.Markdown)
	}
	if data.Metadata.SourceURL != "https://example.com" || data.Metadata.StatusCode != 200 {
		t.Fatalf("metadata = %+v", data.Metadata)
	}

	mu.Lock()
	defer mu.Unlock()
	got := reqs[0]
	if got.Method != http.MethodPost || got.Path != "/v2/scrape" {
		t.Fatalf("request line = %s %s", got.Method, got.Path)
	}
	if got.Auth != "Bearer test-key" {
		t.Fatalf("auth = %q", got.Auth)
	}
	if got.Accept != "application/json" || got.CType != "application/json" {
		t.Fatalf("headers accept=%q content-type=%q", got.Accept, got.CType)
	}
}

func TestScrape_MetadataError(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"markdown":"","html":null,"rawHtml":null,"links":[],"metadata":{"sourceURL":"https://example.com","url":"https://example.com","statusCode":403,"error":"forbidden"},"warning":null}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	client := newClientWithBaseURL(srv.URL, "k")
	data, err := client.Scrape(context.Background(), ScrapeParams{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("client Scrape should not error on metadata error: %v", err)
	}
	if data.Metadata.Error != "forbidden" || data.Metadata.StatusCode != 403 {
		t.Fatalf("metadata = %+v", data.Metadata)
	}
}

func TestScrape_SuccessFalse(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":false,"error":"bad request"}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	client := newClientWithBaseURL(srv.URL, "k")
	_, err := client.Scrape(context.Background(), ScrapeParams{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error on success:false")
	}
	if !strings.Contains(err.Error(), "bad request") {
		t.Fatalf("error should include error field: %v", err)
	}
}

func TestScrape_4xxBodyInError(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"error":"payment required"}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusPaymentRequired, raw)
	})
	t.Cleanup(srv.Close)

	client := newClientWithBaseURL(srv.URL, "k")
	_, err := client.Scrape(context.Background(), ScrapeParams{URL: "https://example.com"})
	if err == nil {
		t.Fatal("expected error on 402")
	}
	if !strings.Contains(err.Error(), "402") || !strings.Contains(err.Error(), "payment required") {
		t.Fatalf("error should include status and body: %v", err)
	}

	mu.Lock()
	defer mu.Unlock()
	if len(reqs) != 1 {
		t.Fatalf("4xx must not retry, got %d requests", len(reqs))
	}
}

func TestScrape_5xxRetryThenSuccess(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const okRaw = `{"success":true,"data":{"markdown":"# Recovered","metadata":{"sourceURL":"https://example.com","statusCode":200,"error":null}}}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		n := len(reqs)
		mu.Unlock()
		if n == 1 {
			writeJSON(w, http.StatusInternalServerError, `{"success":false,"code":"internal","error":"boom"}`)
			return
		}
		writeJSON(w, http.StatusOK, okRaw)
	})
	t.Cleanup(srv.Close)

	client := newClientWithBaseURL(srv.URL, "k")
	data, err := client.Scrape(context.Background(), ScrapeParams{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Scrape() error after retry: %v", err)
	}
	if data.Markdown != "# Recovered" {
		t.Fatalf("markdown = %q", data.Markdown)
	}
	mu.Lock()
	defer mu.Unlock()
	if len(reqs) != 2 {
		t.Fatalf("expected 2 requests (retry once), got %d", len(reqs))
	}
}

func TestSearch_Mapping(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"web":[{"title":"One","description":"first","url":"https://a.com"},{"title":"Two","description":"second","url":"https://b.com"}]},"creditsUsed":2}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	client := newClientWithBaseURL(srv.URL, "k")
	limit := 5
	results, err := client.Search(context.Background(), SearchParams{Query: "golang", Limit: &limit})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 2 || results[0].URL != "https://a.com" || results[1].Title != "Two" {
		t.Fatalf("results = %+v", results)
	}

	mu.Lock()
	defer mu.Unlock()
	if reqs[0].Path != "/v2/search" {
		t.Fatalf("path = %s", reqs[0].Path)
	}
	var sent map[string]any
	if err := json.Unmarshal([]byte(reqs[0].Body), &sent); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if sent["query"] != "golang" {
		t.Fatalf("query = %v", sent["query"])
	}
	if sent["limit"].(float64) != 5 {
		t.Fatalf("limit = %v", sent["limit"])
	}
}

func TestSearch_Empty(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"data":{"web":[]},"creditsUsed":1}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	client := newClientWithBaseURL(srv.URL, "k")
	results, err := client.Search(context.Background(), SearchParams{Query: "nothing"})
	if err != nil {
		t.Fatalf("Search() error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected empty, got %d", len(results))
	}
}

func TestCrawl_FullFlow(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const startRaw = `{"success":true,"id":"job-1","url":"https://api.firecrawl.dev/v2/crawl/job-1"}`
	const scrapingRaw = `{"status":"scraping","total":2,"completed":0,"creditsUsed":0,"expiresAt":"","next":null,"data":[]}`
	const completedRaw = `{"status":"completed","total":2,"completed":2,"creditsUsed":10,"expiresAt":"","next":null,"data":[{"markdown":"# Page A","html":null,"metadata":{"sourceURL":"https://example.com/a","url":"https://example.com/a","statusCode":200,"error":null}}]}`

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

	client := newClientWithBaseURL(srv.URL, "k")
	client.pollInterval = time.Millisecond
	id, err := client.StartCrawl(context.Background(), CrawlParams{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("StartCrawl() error: %v", err)
	}
	if id != "job-1" {
		t.Fatalf("id = %q", id)
	}
	statusURL := client.CrawlStatusURL(id)
	if !strings.HasSuffix(statusURL, "/v2/crawl/job-1") {
		t.Fatalf("statusURL = %q", statusURL)
	}

	first, err := client.CrawlStatus(context.Background(), statusURL)
	if err != nil {
		t.Fatalf("CrawlStatus() error: %v", err)
	}
	if first.Status != "scraping" {
		t.Fatalf("first status = %q", first.Status)
	}
	second, err := client.CrawlStatus(context.Background(), statusURL)
	if err != nil {
		t.Fatalf("CrawlStatus() error: %v", err)
	}
	if second.Status != "completed" || len(second.Data) != 1 || second.Data[0].Markdown != "# Page A" {
		t.Fatalf("second = %+v", second)
	}
}

func TestCrawlStatus_FailedStatus(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"status":"failed","total":0,"completed":0,"creditsUsed":0,"expiresAt":"","next":null,"data":[]}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	client := newClientWithBaseURL(srv.URL, "k")
	status, err := client.CrawlStatus(context.Background(), client.CrawlStatusURL("x"))
	if err != nil {
		t.Fatalf("CrawlStatus() error: %v", err)
	}
	if status.Status != "failed" {
		t.Fatalf("status = %q", status.Status)
	}
}

func TestMap_ObjectLinks(t *testing.T) {
	var mu sync.Mutex
	var reqs []capturedRequest
	const raw = `{"success":true,"links":[{"url":"https://example.com/about","title":"About","description":"a"},{"url":"https://example.com/contact","title":"Contact","description":""}]}`
	srv := newCapture(t, &mu, &reqs, func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, raw)
	})
	t.Cleanup(srv.Close)

	client := newClientWithBaseURL(srv.URL, "k")
	links, err := client.Map(context.Background(), MapParams{URL: "https://example.com"})
	if err != nil {
		t.Fatalf("Map() error: %v", err)
	}
	if len(links) != 2 || links[0].URL != "https://example.com/about" || links[0].Title != "About" {
		t.Fatalf("links = %+v", links)
	}
}
