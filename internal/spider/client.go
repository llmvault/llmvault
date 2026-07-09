package spider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://api.spider.cloud"

// Client communicates with the Spider.cloud REST API.
type Client struct {
	endpoint   string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a Spider.cloud API client.
func NewClient(apiKey string) *Client {
	return newClientWithEndpoint(baseURL, apiKey)
}

func newClientWithEndpoint(endpoint, apiKey string) *Client {
	return &Client{
		endpoint: endpoint,
		apiKey:   apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Crawl crawls a website starting from the given URL.
// POST /v1/crawl
func (client *Client) Crawl(ctx context.Context, params SpiderParams) ([]Response, error) {
	return client.doPost(ctx, "/v1/crawl", params)
}

// Search performs a web search and optionally fetches page content.
// POST /v1/search — returns {"content": [...]} with title/description/url per result.
func (client *Client) Search(ctx context.Context, params SearchParams) (*SearchResponse, error) {
	return doPostJSON[SearchResponse](client, ctx, "/v1/search", params)
}

// Links retrieves all links from the given URL.
// POST /v1/links
func (client *Client) Links(ctx context.Context, params SpiderParams) ([]Response, error) {
	return client.doPost(ctx, "/v1/links", params)
}

// Screenshot takes a screenshot of the given URL.
// POST /v1/screenshot
func (client *Client) Screenshot(ctx context.Context, params SpiderParams) ([]Response, error) {
	return client.doPost(ctx, "/v1/screenshot", params)
}

// Transform converts HTML content to markdown or text without re-fetching.
// POST /v1/transform — returns {"content": ["markdown string", ...]}.
func (client *Client) Transform(ctx context.Context, params TransformParams) (*TransformResponse, error) {
	return doPostJSON[TransformResponse](client, ctx, "/v1/transform", params)
}

// Scrape fetches a single URL and returns its markdown content. It POSTs to
// /v1/crawl with limit=1 so no links are followed — one page in, one page out.
func (client *Client) Scrape(ctx context.Context, pageURL string) (Response, error) {
	limit := 1
	results, err := client.doPost(ctx, "/v1/crawl", SpiderParams{
		URL:          pageURL,
		Limit:        &limit,
		ReturnFormat: "markdown",
		RequestType:  "smart",
		Readability:  boolPtr(true),
	})
	if err != nil {
		return Response{}, err
	}
	if len(results) == 0 {
		return Response{}, fmt.Errorf("spider scrape: empty response for %s", pageURL)
	}
	return results[0], nil
}

func (client *Client) doPost(ctx context.Context, path string, body any) ([]Response, error) {
	respBody, err := client.execute(ctx, path, body)
	if err != nil {
		return nil, err
	}
	var results []Response
	if err := json.Unmarshal(respBody, &results); err != nil {
		return nil, fmt.Errorf("decoding response: %w (body: %s)", err, truncate(string(respBody), 500))
	}
	return results, nil
}

// doPostJSON is a generic helper for endpoints that return non-array responses.
func doPostJSON[T any](client *Client, ctx context.Context, path string, body any) (*T, error) {
	respBody, err := client.execute(ctx, path, body)
	if err != nil {
		return nil, err
	}
	var result T
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w (body: %s)", err, truncate(string(respBody), 500))
	}
	return &result, nil
}

// execute POSTs body to path and returns the raw response bytes, retrying once
// on a transient failure (transport error or 5xx). 4xx returns immediately.
func (client *Client) execute(ctx context.Context, path string, body any) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}

	const maxAttempts = 2
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint+path, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+client.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := client.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("executing request: %w", err)
			continue // transport error — retry once
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("reading response body: %w", readErr)
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("spider API error %d: %s", resp.StatusCode, string(respBody))
			continue // server error — retry once
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("spider API error %d: %s", resp.StatusCode, string(respBody))
		}
		return respBody, nil
	}
	return nil, lastErr
}

func boolPtr(b bool) *bool { return &b }

func truncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen] + "..."
}
