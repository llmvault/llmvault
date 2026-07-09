// Package firecrawl is a REST client for the Firecrawl v2 API plus an adapter
// implementing the webcrawl.Provider contract.
package firecrawl

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const defaultBaseURL = "https://api.firecrawl.dev"

// Client communicates with the Firecrawl v2 REST API.
type Client struct {
	baseURL      string
	apiKey       string
	httpClient   *http.Client
	pollInterval time.Duration
}

// NewClient creates a Firecrawl v2 API client with a 60s HTTP timeout and a 2s
// crawl-status poll interval.
func NewClient(apiKey string) *Client {
	return newClientWithBaseURL(defaultBaseURL, apiKey)
}

func newClientWithBaseURL(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
		pollInterval: 2 * time.Second,
	}
}

// Scrape fetches a single URL. POST /v2/scrape.
func (client *Client) Scrape(ctx context.Context, params ScrapeParams) (*ScrapeData, error) {
	body, err := client.executePost(ctx, "/v2/scrape", params)
	if err != nil {
		return nil, err
	}
	resp, err := decodeResponse[ScrapeResponse](body)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("firecrawl scrape failed: %s", resp.Error)
	}
	return &resp.Data, nil
}

// Search performs a web search. POST /v2/search.
func (client *Client) Search(ctx context.Context, params SearchParams) ([]WebResult, error) {
	body, err := client.executePost(ctx, "/v2/search", params)
	if err != nil {
		return nil, err
	}
	resp, err := decodeResponse[SearchResponse](body)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("firecrawl search failed: %s", resp.Error)
	}
	return resp.Data.Web, nil
}

// StartCrawl begins an asynchronous crawl and returns its job id. POST /v2/crawl.
func (client *Client) StartCrawl(ctx context.Context, params CrawlParams) (string, error) {
	body, err := client.executePost(ctx, "/v2/crawl", params)
	if err != nil {
		return "", err
	}
	resp, err := decodeResponse[StartCrawlResponse](body)
	if err != nil {
		return "", err
	}
	if !resp.Success {
		return "", fmt.Errorf("firecrawl crawl failed: %s", resp.Error)
	}
	if resp.ID == "" {
		return "", fmt.Errorf("firecrawl crawl: empty job id")
	}
	return resp.ID, nil
}

// CrawlStatus fetches a crawl's status from a full URL. The initial URL is built
// with CrawlStatusURL; the response's non-empty "next" field is passed back here
// for pagination. GET {statusURL}.
func (client *Client) CrawlStatus(ctx context.Context, statusURL string) (*CrawlStatusResponse, error) {
	body, err := client.executeGet(ctx, statusURL)
	if err != nil {
		return nil, err
	}
	resp, err := decodeResponse[CrawlStatusResponse](body)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// CrawlStatusURL builds the initial status URL for a crawl job id.
func (client *Client) CrawlStatusURL(id string) string {
	return client.baseURL + "/v2/crawl/" + id
}

// Map enumerates URLs reachable from a site. POST /v2/map.
func (client *Client) Map(ctx context.Context, params MapParams) ([]MapLink, error) {
	body, err := client.executePost(ctx, "/v2/map", params)
	if err != nil {
		return nil, err
	}
	resp, err := decodeResponse[MapResponse](body)
	if err != nil {
		return nil, err
	}
	if !resp.Success {
		return nil, fmt.Errorf("firecrawl map failed: %s", resp.Error)
	}
	return resp.Links, nil
}

func (client *Client) executePost(ctx context.Context, path string, body any) ([]byte, error) {
	jsonBody, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshaling request body: %w", err)
	}
	return client.execute(ctx, http.MethodPost, client.baseURL+path, jsonBody)
}

func (client *Client) executeGet(ctx context.Context, url string) ([]byte, error) {
	return client.execute(ctx, http.MethodGet, url, nil)
}

// execute performs an HTTP request and returns the raw response bytes, retrying
// once on a transient failure (transport error or 5xx). 4xx returns immediately
// with the response body included in the error.
func (client *Client) execute(ctx context.Context, method, url string, jsonBody []byte) ([]byte, error) {
	const maxAttempts = 2
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		var reader io.Reader
		if jsonBody != nil {
			reader = bytes.NewReader(jsonBody)
		}
		req, err := http.NewRequestWithContext(ctx, method, url, reader)
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
			lastErr = fmt.Errorf("firecrawl API error %d: %s", resp.StatusCode, truncate(string(respBody), 500))
			continue // server error — retry once
		}
		if resp.StatusCode >= 400 {
			return nil, fmt.Errorf("firecrawl API error %d: %s", resp.StatusCode, truncate(string(respBody), 500))
		}
		return respBody, nil
	}
	return nil, lastErr
}

func decodeResponse[T any](body []byte) (*T, error) {
	var result T
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w (body: %s)", err, truncate(string(body), 500))
	}
	return &result, nil
}

func truncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen] + "..."
}
