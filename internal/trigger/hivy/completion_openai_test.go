package hivy

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

type captureDoer struct {
	req *http.Request
}

func (d *captureDoer) Do(req *http.Request) (*http.Response, error) {
	d.req = req
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
	}, nil
}

func TestOpenRouterHeaderDoerUsesHivyAppHeaders(t *testing.T) {
	inner := &captureDoer{}
	doer := openRouterHeaderDoer{inner: inner}
	req, err := http.NewRequest(http.MethodPost, "https://openrouter.ai/api/v1/chat/completions", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := doer.Do(req)
	if err != nil {
		t.Fatalf("Do: %v", err)
	}
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
	if inner.req == nil {
		t.Fatal("inner doer was not called")
	}
	if got := inner.req.Header.Get("HTTP-Referer"); got != "https://usehivy.com" {
		t.Fatalf("HTTP-Referer = %q, want https://usehivy.com", got)
	}
	if got := inner.req.Header.Get("X-Title"); got != "Hivy" {
		t.Fatalf("X-Title = %q, want Hivy", got)
	}
}
