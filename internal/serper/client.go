// Package serper is a REST client for the Serper.dev Google search API plus
// a search-only adapter implementing the webcrawl.Provider contract.
package serper

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const baseURL = "https://google.serper.dev"

// Client communicates with the Serper.dev REST API.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewClient creates a Serper.dev API client.
func NewClient(apiKey string) *Client {
	return newClientWithBaseURL(baseURL, apiKey)
}

func newClientWithBaseURL(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 60 * time.Second,
		},
	}
}

// Search performs a Google web search. POST /search.
func (client *Client) Search(ctx context.Context, params SearchParams) ([]OrganicResult, error) {
	respBody, err := client.execute(ctx, "/search", params)
	if err != nil {
		return nil, err
	}
	var result SearchResponse
	if err := json.Unmarshal(respBody, &result); err != nil {
		return nil, fmt.Errorf("decoding response: %w (body: %s)", err, truncate(string(respBody), 500))
	}
	return result.Organic, nil
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
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, client.baseURL+path, bytes.NewReader(jsonBody))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("X-API-KEY", client.apiKey)
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		resp, err := client.httpClient.Do(req)
		if err != nil {
			lastErr = fmt.Errorf("executing request: %w", err)
			continue
		}
		respBody, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("reading response body: %w", readErr)
		}
		if resp.StatusCode >= 500 {
			lastErr = apiError(resp.StatusCode, respBody)
			continue
		}
		if resp.StatusCode >= 400 {
			return nil, apiError(resp.StatusCode, respBody)
		}
		return respBody, nil
	}
	return nil, lastErr
}

func apiError(status int, body []byte) error {
	var er errorResponse
	if err := json.Unmarshal(body, &er); err == nil && er.Message != "" {
		return fmt.Errorf("serper API error %d: %s", status, er.Message)
	}
	return fmt.Errorf("serper API error %d: %s", status, truncate(string(body), 500))
}

func truncate(str string, maxLen int) string {
	if len(str) <= maxLen {
		return str
	}
	return str[:maxLen] + "..."
}
