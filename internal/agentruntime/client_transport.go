package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

func (c *Client) doVoid(ctx context.Context, method, path string, body any) error {
	resp, err := c.do(ctx, method, path, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, raw)
	}
	return nil
}

func (c *Client) do(ctx context.Context, method, path string, body any) (*http.Response, error) {
	return c.doRuntimeRequest(ctx, method, path, body, true)
}

func (c *Client) doRuntimeRequest(ctx context.Context, method, path string, body any, auth bool) (*http.Response, error) {
	var data []byte
	if body != nil {
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
	}
	req, err := c.newRequest(ctx, method, c.baseURL+path, data, auth)
	if err != nil {
		return nil, err
	}
	resp, err := c.http.Do(req)
	return resp, err
}

func (c *Client) newRequest(ctx context.Context, method, rawURL string, data []byte, auth bool) (*http.Request, error) {
	var reader io.Reader
	if data != nil {
		reader = bytes.NewReader(data)
	}
	req, err := http.NewRequestWithContext(ctx, method, rawURL, reader)
	if err != nil {
		return nil, err
	}
	if auth {
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
	}
	if data != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}
