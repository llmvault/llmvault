package website

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
	"github.com/usehivy/hivy/internal/webcrawl"
)

type stubSource struct{ cfg json.RawMessage }

func (s *stubSource) SourceID() string        { return "src-1" }
func (s *stubSource) OrgID() string           { return "org-1" }
func (s *stubSource) SourceKind() string      { return Kind }
func (s *stubSource) Config() json.RawMessage { return s.cfg }

// fakeProvider serves a canned Scrape result (or error) per URL. Page-level
// failures and non-2xx statuses arrive as errors, matching how the real
// provider adapters surface them.
type fakeProvider struct {
	pages map[string]webcrawl.Page
	errs  map[string]error
}

func (f *fakeProvider) Name() string { return "fake" }

func (f *fakeProvider) Scrape(_ context.Context, req webcrawl.ScrapeRequest) (webcrawl.Page, error) {
	if err, ok := f.errs[req.URL]; ok {
		return webcrawl.Page{}, err
	}
	if p, ok := f.pages[req.URL]; ok {
		return p, nil
	}
	return webcrawl.Page{}, fmt.Errorf("fake: no page for %s", req.URL)
}

func (f *fakeProvider) Crawl(context.Context, webcrawl.CrawlRequest) ([]webcrawl.Page, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *fakeProvider) Search(context.Context, webcrawl.SearchRequest) ([]webcrawl.SearchResult, error) {
	return nil, fmt.Errorf("not implemented")
}

func (f *fakeProvider) Map(context.Context, webcrawl.MapRequest) ([]string, error) {
	return nil, fmt.Errorf("not implemented")
}

func TestRun_ScrapesEachURL(t *testing.T) {
	// One scrape per configured URL: /  -> doc, /a -> empty content (skipped),
	// /b -> page-level error (failure), /c -> doc.
	web := &fakeProvider{
		pages: map[string]webcrawl.Page{
			"https://example.com/":  {URL: "https://example.com/", Content: "# Home", StatusCode: 200},
			"https://example.com/a": {URL: "https://example.com/a", Content: "", StatusCode: 200},
			"https://example.com/c": {URL: "https://example.com/c", Content: "## C body", StatusCode: 200},
		},
		errs: map[string]error{
			"https://example.com/b": fmt.Errorf("scrape https://example.com/b: status 500"),
		},
	}

	c := NewConnector(WebsiteConfig{URLs: []string{
		"https://example.com/",
		"https://example.com/a",
		"https://example.com/b",
		"https://example.com/c",
	}}, web)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := c.Run(ctx, &stubSource{}, nil, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var docs []*interfaces.Document
	var fails []*interfaces.ConnectorFailure
	for ev := range out {
		if ev.Doc != nil {
			docs = append(docs, ev.Doc)
		}
		if ev.Failure != nil {
			fails = append(fails, ev.Failure)
		}
	}
	if len(docs) != 2 {
		t.Fatalf("docs: got %d, want 2 (URLs: %v)", len(docs), urlsOf(docs))
	}
	if len(fails) != 1 {
		t.Fatalf("failures: got %d, want 1", len(fails))
	}
	if got := docs[0].DocID; got != "https://example.com/" {
		t.Errorf("docs[0].DocID = %q, want %q", got, "https://example.com/")
	}
	if got := docs[1].DocID; got != "https://example.com/c" {
		t.Errorf("docs[1].DocID = %q, want %q", got, "https://example.com/c")
	}
	if fails[0].FailedDocument == nil || fails[0].FailedDocument.DocID != "https://example.com/b" {
		t.Errorf("failure didn't pin to /b: %+v", fails[0])
	}
}

func TestRun_URLFallbackAndMaxPages(t *testing.T) {
	// A page with empty URL falls back to the requested URL; MaxPages caps the
	// list so the second URL is never scraped.
	web := &fakeProvider{
		pages: map[string]webcrawl.Page{
			"https://example.com/one": {URL: "", Content: "# One", StatusCode: 200},
			"https://example.com/two": {URL: "https://example.com/two", Content: "# Two", StatusCode: 200},
		},
	}

	c := NewConnector(WebsiteConfig{
		URLs:     []string{"https://example.com/one", "https://example.com/two"},
		MaxPages: 1,
	}, web)

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	out, err := c.Run(ctx, &stubSource{}, nil, time.Time{}, time.Time{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	var docs []*interfaces.Document
	for ev := range out {
		if ev.Doc != nil {
			docs = append(docs, ev.Doc)
		}
	}
	if len(docs) != 1 {
		t.Fatalf("docs: got %d, want 1 (MaxPages cap) (%v)", len(docs), urlsOf(docs))
	}
	if got := docs[0].Link; got != "https://example.com/one" {
		t.Errorf("URL fallback failed: Link = %q, want %q", got, "https://example.com/one")
	}
}

func urlsOf(docs []*interfaces.Document) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.DocID
	}
	return out
}
