package agentruntime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
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
	if err == nil {
		return resp, nil
	}
	fallbackBase, ok := localDockerHostBaseURL(c.baseURL)
	if !ok {
		return nil, err
	}
	fallbackReq, fallbackReqErr := c.newRequest(ctx, method, fallbackBase+path, data, auth)
	if fallbackReqErr != nil {
		return nil, err
	}
	fallbackResp, fallbackErr := c.http.Do(fallbackReq)
	if fallbackErr != nil {
		return nil, err
	}
	return fallbackResp, nil
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

func localDockerHostBaseURL(raw string) (string, bool) {
	parsed, err := url.Parse(raw)
	if err != nil {
		return "", false
	}
	host := parsed.Hostname()
	switch {
	case host == "host.docker.internal":
		if port := parsed.Port(); port != "" {
			parsed.Host = net.JoinHostPort("localhost", port)
		} else {
			parsed.Host = "localhost"
		}
	case isLocalDockerHost(host):
		if port := parsed.Port(); port != "" {
			parsed.Host = net.JoinHostPort("host.docker.internal", port)
		} else {
			parsed.Host = "host.docker.internal"
		}
	default:
		return "", false
	}
	return strings.TrimRight(parsed.String(), "/"), true
}

func isLocalDockerHost(host string) bool {
	if host == "localhost" {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
