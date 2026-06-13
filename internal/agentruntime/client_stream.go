package agentruntime

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

func (c *Client) StreamHTTP(ctx context.Context, path string) (*http.Response, error) {
	if strings.TrimSpace(path) == "" {
		return nil, fmt.Errorf("runtime stream path is required")
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

// streamClient returns the untimed SSE client, falling back to the general one.
func (c *Client) streamClient() *http.Client {
	if c.stream != nil {
		return c.stream
	}
	return c.http
}

func (c *Client) openStream(ctx context.Context, rawURL string) (*http.Response, error) {
	req, err := c.newRequest(ctx, http.MethodGet, rawURL, nil, true)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Accept-Encoding", "identity")
	// Use the dedicated streaming client (no overall Timeout) so long agent
	// turns are not cut mid-body at the default 2-minute boundary.
	resp, err := c.streamClient().Do(req)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		resp.Body.Close()
		return nil, fmt.Errorf("runtime stream: %s: %s", resp.Status, strings.TrimSpace(string(raw)))
	}
	return resp, nil
}
