package website

import (
	"context"
	"testing"

	"github.com/usehivy/hivy/internal/rag/connectors/interfaces"
)

func TestListAllSlim_OneDocPerURL(t *testing.T) {
	urls := []string{"https://acme.com/library", "https://acme.com/blog"}
	c := NewConnector(WebsiteConfig{URLs: urls}, nil) // slim never touches spider

	ch, err := c.ListAllSlim(context.Background(), &stubSource{})
	if err != nil {
		t.Fatalf("ListAllSlim: %v", err)
	}

	got := map[string]bool{}
	for ev := range ch {
		if ev.Failure != nil {
			t.Fatalf("unexpected slim failure: %+v", ev.Failure)
		}
		if ev.Slim != nil {
			got[ev.Slim.DocID] = true
		}
	}
	if len(got) != len(urls) {
		t.Fatalf("slim doc count = %d, want %d (%v)", len(got), len(urls), got)
	}
	for _, u := range urls {
		if !got[canonicalURL(u)] {
			t.Fatalf("slim missing doc id for %s (canonical %s); got %v", u, canonicalURL(u), got)
		}
	}
}

var _ interfaces.SlimConnector = (*WebsiteConnector)(nil)
