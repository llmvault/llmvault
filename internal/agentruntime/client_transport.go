package agentruntime

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/usehivy/hivy/internal/logging"
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

func (c *Client) doGzip(ctx context.Context, method, path string, body any) (*http.Response, error) {
	prepareStarted := time.Now()
	var data []byte
	var encodeDuration time.Duration
	if body != nil {
		encodeStarted := time.Now()
		var err error
		data, err = json.Marshal(body)
		if err != nil {
			return nil, err
		}
		encodeDuration = time.Since(encodeStarted)
	}
	compressStarted := time.Now()
	compressed, err := gzipRequestBody(data)
	if err != nil {
		return nil, err
	}
	compressDuration := time.Since(compressStarted)
	logging.FromContext(ctx).InfoContext(ctx, "runtime config request compressed",
		"json_body_bytes", len(data),
		"wire_body_bytes", len(compressed),
		"compression_ratio_percent", compressionRatioPercent(len(compressed), len(data)),
		"json_encode_ms", encodeDuration.Milliseconds(),
		"gzip_ms", compressDuration.Milliseconds(),
		"prepare_ms", time.Since(prepareStarted).Milliseconds(),
	)
	req, err := c.newRequest(ctx, method, c.baseURL+path, compressed, true)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Encoding", "gzip")
	return c.http.Do(req)
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

func gzipRequestBody(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	writer, err := gzip.NewWriterLevel(&buf, gzip.BestSpeed)
	if err != nil {
		return nil, fmt.Errorf("create gzip writer: %w", err)
	}
	if _, err := writer.Write(data); err != nil {
		_ = writer.Close()
		return nil, fmt.Errorf("gzip request body: %w", err)
	}
	if err := writer.Close(); err != nil {
		return nil, fmt.Errorf("finish gzip request body: %w", err)
	}
	return buf.Bytes(), nil
}

func compressionRatioPercent(compressed, original int) int {
	if original <= 0 {
		return 0
	}
	return compressed * 100 / original
}
