package employeeruntime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c *Client) StreamHTTP(ctx context.Context, path string) (*http.Response, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("stream http gateway: path is required")
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	resp, err := c.openStream(ctx, c.baseURL+path)
	if err == nil {
		return resp, nil
	}
	fallbackBase, ok := localDockerHostBaseURL(c.baseURL)
	if !ok {
		return nil, err
	}
	return c.openStream(ctx, fallbackBase+path)
}

func (c *Client) openStream(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := c.newRequest(ctx, http.MethodGet, rawURL, nil, true)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Encoding", "identity")
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("stream http gateway: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return resp, nil
}
