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

func decodeBody(t *testing.T, body string) map[string]any {
	t.Helper()
	var sent map[string]any
	if err := json.Unmarshal([]byte(body), &sent); err != nil {
		t.Fatalf("unmarshal sent body: %v", err)
	}
	return sent
}
