package webcrawl

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func callTool(handler mcp.ToolHandler, args map[string]any) (*mcp.CallToolResult, error) {
	var raw json.RawMessage
	if args != nil {
		raw, _ = json.Marshal(args)
	}
	return handler(context.Background(), &mcp.CallToolRequest{
		Params: &mcp.CallToolParamsRaw{
			Arguments: raw,
		},
	})
}

func textContent(result *mcp.CallToolResult) string {
	if len(result.Content) == 0 {
		return ""
	}
	if tc, ok := result.Content[0].(*mcp.TextContent); ok {
		return tc.Text
	}
	return ""
}

func TestWebFetchHandler_Success(t *testing.T) {
	var gotReq ScrapeRequest
	provider := &fakeProvider{
		name: "fake",
		scrape: func(_ context.Context, req ScrapeRequest) (Page, error) {
			gotReq = req
			return Page{URL: "https://example.com", Content: "# Hello World", StatusCode: 200}, nil
		},
	}

	result, err := callTool(WebFetchHandler(provider), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result))
	}

	var body map[string]any
	if err := json.Unmarshal([]byte(textContent(result)), &body); err != nil {
		t.Fatalf("unmarshal result: %v", err)
	}
	if body["url"] != "https://example.com" {
		t.Errorf("expected url 'https://example.com', got %v", body["url"])
	}
	if body["content"] != "# Hello World" {
		t.Errorf("expected content '# Hello World', got %v", body["content"])
	}
	if body["status"] != float64(200) {
		t.Errorf("expected status 200, got %v", body["status"])
	}

	if gotReq.URL != "https://example.com" {
		t.Errorf("expected request url 'https://example.com', got %q", gotReq.URL)
	}
	if gotReq.Format != FormatMarkdown {
		t.Errorf("expected default format markdown, got %q", gotReq.Format)
	}
}

func TestWebFetchHandler_ExplicitFormat(t *testing.T) {
	var gotReq ScrapeRequest
	provider := &fakeProvider{
		name: "fake",
		scrape: func(_ context.Context, req ScrapeRequest) (Page, error) {
			gotReq = req
			return Page{URL: "https://example.com", Content: "<h1>Hi</h1>", StatusCode: 200}, nil
		},
	}

	result, err := callTool(WebFetchHandler(provider), map[string]any{
		"url":           "https://example.com",
		"return_format": "html",
	})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result))
	}
	if gotReq.Format != FormatHTML {
		t.Errorf("expected format html, got %q", gotReq.Format)
	}
}

func TestWebFetchHandler_MissingURL(t *testing.T) {
	provider := &fakeProvider{name: "fake", scrape: func(context.Context, ScrapeRequest) (Page, error) {
		t.Fatal("scrape should not be called")
		return Page{}, nil
	}}

	result, err := callTool(WebFetchHandler(provider), map[string]any{})
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

func TestWebFetchHandler_ProviderError(t *testing.T) {
	provider := &fakeProvider{name: "fake", scrape: func(context.Context, ScrapeRequest) (Page, error) {
		return Page{}, errors.New("boom")
	}}

	result, err := callTool(WebFetchHandler(provider), map[string]any{"url": "https://example.com"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error, got success")
	}
	if textContent(result) != "Error: web fetch failed: boom" {
		t.Errorf("unexpected error text: %q", textContent(result))
	}
}

func TestWebSearchHandler_Success(t *testing.T) {
	var gotReq SearchRequest
	provider := &fakeProvider{
		name: "fake",
		search: func(_ context.Context, req SearchRequest) ([]SearchResult, error) {
			gotReq = req
			return []SearchResult{
				{Title: "Go Testing", URL: "https://go.dev/doc/testing", Description: "Official Go testing docs"},
				{Title: "Testify", URL: "https://github.com/stretchr/testify", Description: "Testing toolkit"},
			}, nil
		},
	}

	num := 3
	result, err := callTool(WebSearchHandler(provider), map[string]any{"query": "golang testing", "num": num})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if result.IsError {
		t.Fatalf("expected success, got error: %s", textContent(result))
	}

	var results []SearchResult
	if err := json.Unmarshal([]byte(textContent(result)), &results); err != nil {
		t.Fatalf("unmarshal results: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].Title != "Go Testing" {
		t.Errorf("expected first result title 'Go Testing', got %q", results[0].Title)
	}

	if gotReq.Query != "golang testing" {
		t.Errorf("expected query 'golang testing', got %q", gotReq.Query)
	}
	if gotReq.Limit != 3 {
		t.Errorf("expected limit 3, got %d", gotReq.Limit)
	}
}

func TestWebSearchHandler_MissingQuery(t *testing.T) {
	provider := &fakeProvider{name: "fake", search: func(context.Context, SearchRequest) ([]SearchResult, error) {
		t.Fatal("search should not be called")
		return nil, nil
	}}

	result, err := callTool(WebSearchHandler(provider), map[string]any{})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error for missing query, got success")
	}
	if textContent(result) != "Error: query is required" {
		t.Errorf("expected 'Error: query is required', got %q", textContent(result))
	}
}

func TestWebSearchHandler_ProviderError(t *testing.T) {
	provider := &fakeProvider{name: "fake", search: func(context.Context, SearchRequest) ([]SearchResult, error) {
		return nil, errors.New("boom")
	}}

	result, err := callTool(WebSearchHandler(provider), map[string]any{"query": "q"})
	if err != nil {
		t.Fatalf("handler error: %v", err)
	}
	if !result.IsError {
		t.Fatal("expected error, got success")
	}
	if textContent(result) != "Error: web search failed: boom" {
		t.Errorf("unexpected error text: %q", textContent(result))
	}
}
