package firecrawl

import (
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
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
