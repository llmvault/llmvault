package serper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/usehivy/hivy/internal/webcrawl"
)

func TestProviderName(t *testing.T) {
	if got := NewProvider(NewClient("k")).Name(); got != "serper" {
		t.Fatalf("Name() = %q, want serper", got)
	}
}

func TestProviderSearch(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(searchFixture))
	}))
	defer srv.Close()

	provider := NewProvider(newClientWithBaseURL(srv.URL, "k"))
	results, err := provider.Search(context.Background(), webcrawl.SearchRequest{Query: "apple inc", Limit: 3})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["num"] != float64(3) {
		t.Errorf("body num = %v, want 3", gotBody["num"])
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	want := webcrawl.SearchResult{
		URL:         "https://en.wikipedia.org/wiki/Apple_Inc.",
		Title:       "Apple Inc. - Wikipedia",
		Description: "Apple Inc. is an American multinational technology company headquartered in Cupertino.",
	}
	if results[0] != want {
		t.Errorf("first result = %+v, want %+v", results[0], want)
	}
}

func TestProviderSearchClampsNum(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(searchFixture))
	}))
	defer srv.Close()

	provider := NewProvider(newClientWithBaseURL(srv.URL, "k"))
	if _, err := provider.Search(context.Background(), webcrawl.SearchRequest{Query: "q", Limit: 250}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if gotBody["num"] != float64(100) {
		t.Errorf("body num = %v, want 100", gotBody["num"])
	}
}

func TestProviderSearchZeroHits(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"searchParameters": {"q": "gibberish"}, "credits": 1}`))
	}))
	defer srv.Close()

	provider := NewProvider(newClientWithBaseURL(srv.URL, "k"))
	results, err := provider.Search(context.Background(), webcrawl.SearchRequest{Query: "gibberish"})
	if err != nil {
		t.Fatalf("zero hits must not error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("results = %d, want 0", len(results))
	}
}

func TestProviderUnsupportedOperations(t *testing.T) {
	provider := NewProvider(NewClient("k"))
	if _, err := provider.Scrape(context.Background(), webcrawl.ScrapeRequest{URL: "https://x.test"}); err == nil {
		t.Error("scrape: expected error, got nil")
	}
	if _, err := provider.Crawl(context.Background(), webcrawl.CrawlRequest{URL: "https://x.test"}); err == nil {
		t.Error("crawl: expected error, got nil")
	}
	if _, err := provider.Map(context.Background(), webcrawl.MapRequest{URL: "https://x.test"}); err == nil {
		t.Error("map: expected error, got nil")
	}
}
