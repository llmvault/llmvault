package firecrawl

import (
	"context"
	"net/http"
	"strings"
	"sync"
	"testing"
	"time"
)

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
