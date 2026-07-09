package webcrawl

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

func TestWebCrawlHandler_Success(t *testing.T) {
	var gotReq CrawlRequest
	provider := &fakeProvider{
		name: "fake",
		crawl: func(_ context.Context, req CrawlRequest) ([]Page, error) {
			gotReq = req
			return []Page{
				{URL: "https://example.com", Content: "one", StatusCode: 200},
				{URL: "https://example.com/a", Content: "two", StatusCode: 200},
			}, nil
		},
	}

	result, err := callTool(WebCrawlHandler(provider), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result))
	}

	var pages []map[string]any
	if err := json.Unmarshal([]byte(textContent(result)), &pages); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if len(pages) != 2 {
		t.Fatalf("expected 2 pages, got %d", len(pages))
	}
	if pages[0]["content"] != "one" {
		t.Errorf("expected first page content 'one', got %v", pages[0]["content"])
	}

	if gotReq.Limit != 10 {
		t.Errorf("expected default limit 10, got %d", gotReq.Limit)
	}
	if gotReq.Format != FormatMarkdown {
		t.Errorf("expected default format markdown, got %q", gotReq.Format)
	}
}

func TestWebCrawlHandler_LimitCapped(t *testing.T) {
	var gotReq CrawlRequest
	provider := &fakeProvider{
		name: "fake",
		crawl: func(_ context.Context, req CrawlRequest) ([]Page, error) {
			gotReq = req
			return []Page{{URL: "https://example.com", Content: "c", StatusCode: 200}}, nil
		},
	}

	limit := 500
	result, err := callTool(WebCrawlHandler(provider), map[string]any{"url": "https://example.com", "limit": limit})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result))
	}
	if gotReq.Limit != 100 {
		t.Errorf("expected limit capped at 100, got %d", gotReq.Limit)
	}
}

func TestWebCrawlHandler_MissingURL(t *testing.T) {
	provider := &fakeProvider{name: "fake", crawl: func(context.Context, CrawlRequest) ([]Page, error) {
		t.Fatal("crawl should not be called")
		return nil, nil
	}}

	result, err := callTool(WebCrawlHandler(provider), map[string]any{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing url, got success")
	}
	if textContent(result) != "Error: url is required" {
		t.Errorf("expected 'Error: url is required', got %q", textContent(result))
	}
}

func TestWebCrawlHandler_EmptyResult(t *testing.T) {
	provider := &fakeProvider{name: "fake", crawl: func(context.Context, CrawlRequest) ([]Page, error) {
		return nil, nil
	}}

	result, err := callTool(WebCrawlHandler(provider), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for empty result, got success")
	}
	if textContent(result) != "Error: no content returned for crawl" {
		t.Errorf("expected 'Error: no content returned for crawl', got %q", textContent(result))
	}
}

func TestWebCrawlHandler_ProviderError(t *testing.T) {
	provider := &fakeProvider{name: "fake", crawl: func(context.Context, CrawlRequest) ([]Page, error) {
		return nil, errors.New("boom")
	}}

	result, err := callTool(WebCrawlHandler(provider), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error, got success")
	}
	if textContent(result) != "Error: web crawl failed: boom" {
		t.Errorf("unexpected error text: %q", textContent(result))
	}
}
