package control

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type RunnerClient struct {
	token  string
	client *http.Client
}

func NewRunnerClient(token string) *RunnerClient {
	return &RunnerClient{
		token:  token,
		client: &http.Client{Timeout: 5 * time.Minute},
	}
}

func (c *RunnerClient) Post(ctx context.Context, baseURL, path string, in, out any) error {
	return c.do(ctx, http.MethodPost, baseURL, path, in, out)
}

func (c *RunnerClient) Delete(ctx context.Context, baseURL, path string) error {
	return c.do(ctx, http.MethodDelete, baseURL, path, nil, nil)
}

func (c *RunnerClient) do(ctx context.Context, method, baseURL, path string, in, out any) error {
	var body io.Reader
	if in != nil {
		raw, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(baseURL, "/")+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("runner returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	if out != nil {
		return json.NewDecoder(resp.Body).Decode(out)
	}
	return nil
}

func (c *RunnerClient) Get(ctx context.Context, baseURL, path string, out io.Writer) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimRight(baseURL, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	resp, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return fmt.Errorf("runner returned %d: %s", resp.StatusCode, strings.TrimSpace(string(raw)))
	}
	_, err = io.Copy(out, resp.Body)
	return err
}
