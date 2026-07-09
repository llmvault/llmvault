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
