package serper

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

const searchFixture = `{
  "searchParameters": {"q": "apple inc", "type": "search", "num": 3, "engine": "google"},
  "knowledgeGraph": {"title": "Apple", "description": "Apple Inc. is an American multinational technology company."},
  "organic": [
    {"title": "Apple Inc. - Wikipedia", "link": "https://en.wikipedia.org/wiki/Apple_Inc.", "snippet": "Apple Inc. is an American multinational technology company headquartered in Cupertino.", "position": 1},
    {"title": "Apple", "link": "https://www.apple.com/", "snippet": "Discover the innovative world of Apple.", "sitelinks": [{"title": "Store", "link": "https://www.apple.com/us/shop/goto/store"}], "position": 2}
  ],
  "relatedSearches": [{"query": "Apple Inc full form"}],
  "credits": 1
}`

func TestSearch(t *testing.T) {
	var gotPath, gotKey, gotContentType string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("X-API-KEY")
		gotContentType = r.Header.Get("Content-Type")
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(searchFixture))
	}))
	defer srv.Close()

	client := newClientWithBaseURL(srv.URL, "test-key")
	num := 3
	results, err := client.Search(context.Background(), SearchParams{Query: "apple inc", Num: &num})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if gotPath != "/search" {
		t.Errorf("path = %q, want /search", gotPath)
	}
	if gotKey != "test-key" {
		t.Errorf("X-API-KEY = %q, want test-key", gotKey)
	}
	if gotContentType != "application/json" {
		t.Errorf("Content-Type = %q", gotContentType)
	}
	if gotBody["q"] != "apple inc" {
		t.Errorf("body q = %v", gotBody["q"])
	}
	if gotBody["num"] != float64(3) {
		t.Errorf("body num = %v", gotBody["num"])
	}

	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	first := results[0]
	if first.Title != "Apple Inc. - Wikipedia" || first.Link != "https://en.wikipedia.org/wiki/Apple_Inc." || first.Position != 1 {
		t.Errorf("unexpected first result: %+v", first)
	}
	if !strings.Contains(first.Snippet, "Cupertino") {
		t.Errorf("unexpected snippet: %q", first.Snippet)
	}
}

func TestSearchOmitsNumWhenNil(t *testing.T) {
	var raw []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body := make([]byte, r.ContentLength)
		_, _ = r.Body.Read(body)
		raw = body
		_, _ = w.Write([]byte(`{"organic": []}`))
	}))
	defer srv.Close()

	client := newClientWithBaseURL(srv.URL, "k")
	results, err := client.Search(context.Background(), SearchParams{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 0 {
		t.Fatalf("expected no results, got %d", len(results))
	}
	if strings.Contains(string(raw), "num") {
		t.Errorf("body should omit num: %s", raw)
	}
}

func TestSearchUnauthorized(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(`{"message":"Unauthorized.","statusCode":403}`))
	}))
	defer srv.Close()

	client := newClientWithBaseURL(srv.URL, "bad-key")
	_, err := client.Search(context.Background(), SearchParams{Query: "q"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "403") || !strings.Contains(err.Error(), "Unauthorized.") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestSearchRetriesOn5xx(t *testing.T) {
	attempts := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		attempts++
		if attempts == 1 {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		_, _ = w.Write([]byte(searchFixture))
	}))
	defer srv.Close()

	client := newClientWithBaseURL(srv.URL, "k")
	results, err := client.Search(context.Background(), SearchParams{Query: "q"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if attempts != 2 {
		t.Errorf("attempts = %d, want 2", attempts)
	}
	if len(results) != 2 {
		t.Errorf("results = %d, want 2", len(results))
	}
}

func TestSearchFailsAfterRetries(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadGateway)
		_, _ = w.Write([]byte(`{"message":"upstream broke","statusCode":502}`))
	}))
	defer srv.Close()

	client := newClientWithBaseURL(srv.URL, "k")
	_, err := client.Search(context.Background(), SearchParams{Query: "q"})
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !strings.Contains(err.Error(), "502") || !strings.Contains(err.Error(), "upstream broke") {
		t.Errorf("unexpected error: %v", err)
	}
}
