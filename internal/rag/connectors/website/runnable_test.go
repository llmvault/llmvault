package website

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
	"github.com/usehivy/hivy/internal/spider"
)

type stubSource struct{ cfg json.RawMessage }

func (s *stubSource) SourceID() string        { return "src-1" }
func (s *stubSource) OrgID() string           { return "org-1" }
func (s *stubSource) SourceKind() string      { return Kind }
func (s *stubSource) Config() json.RawMessage { return s.cfg }

func TestRun_ScrapesEachURL(t *testing.T) {
	// One scrape (POST /v1/crawl, limit=1) per configured URL: /  -> doc,
	// /a -> empty content (skipped), /b -> 5xx (failure), /c -> doc.
	byURL := map[string]spider.Response{
		"https://example.com/":  {URL: "https://example.com/", Content: "# Home", StatusCode: 200},
		"https://example.com/a": {URL: "https://example.com/a", Content: "", StatusCode: 200},
		"https://example.com/b": {URL: "https://example.com/b", Content: "", StatusCode: 500, Error: "bad gateway"},
		"https://example.com/c": {URL: "https://example.com/c", Content: "## C body", StatusCode: 200},
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var p spider.SpiderParams
		_ = json.NewDecoder(r.Body).Decode(&p)
		resp, ok := byURL[p.URL]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if resp.StatusCode >= 500 {
			w.WriteHeader(resp.StatusCode)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode([]spider.Response{resp})
	}))
	defer srv.Close()

	cli := spider.NewClient(srv.URL, "k")
	c := NewConnector(WebsiteConfig{URLs: []string{
		"https://example.com/",
		"https://example.com/a",
		"https://example.com/b",
		"https://example.com/c",
	}}, cli)

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

func urlsOf(docs []*interfaces.Document) []string {
	out := make([]string, len(docs))
	for i, d := range docs {
		out[i] = d.DocID
	}
	return out
}
