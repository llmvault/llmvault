package apps

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// appdClient is a small typed client for the hivy-appd daemon inside an app
// sandbox (cmd/hivy-appd). Bearer = the app's decrypted runtime secret.
type appdClient struct {
	http *http.Client
	// bootRetryWindow bounds how long deployWithBootRetry keeps retrying
	// connection-level failures while a freshly created container boots.
	bootRetryWindow time.Duration
	// bootRetryInterval is the pause between boot retries.
	bootRetryInterval time.Duration
}

func newAppdClient() *appdClient {
	return &appdClient{
		// Deploy downloads and extracts the bundle inside the sandbox; give it
		// room. Every other endpoint is quick.
		http:              &http.Client{Timeout: 5 * time.Minute},
		bootRetryWindow:   60 * time.Second,
		bootRetryInterval: 2 * time.Second,
	}
}

// AppdDeployRequest mirrors cmd/hivy-appd deployRequest.
type AppdDeployRequest struct {
	BundleURL string            `json:"bundle_url"`
	SHA256    string            `json:"sha256"`
	VersionID string            `json:"version_id"`
	Env       map[string]string `json:"env,omitempty"`
}

// AppdDeployResponse mirrors cmd/hivy-appd deployResponse.
type AppdDeployResponse struct {
	OldSHA     string `json:"old_sha"`
	NewSHA     string `json:"new_sha"`
	VersionID  string `json:"version_id,omitempty"`
	DownloadMS int64  `json:"download_ms"`
	ExtractMS  int64  `json:"extract_ms"`
	RestartMS  int64  `json:"restart_ms"`
	TotalMS    int64  `json:"total_ms"`
}

// AppdError is a non-2xx appd reply.
type AppdError struct {
	StatusCode int
	Message    string
}

func (e *AppdError) Error() string {
	return fmt.Sprintf("appd: status %d: %s", e.StatusCode, e.Message)
}

// LogsQuery is the passthrough argument set for GET /logs.
type LogsQuery struct {
	Lines  int
	Grep   string
	Since  string // RFC3339
	Stream string // "app" | "appd"
}

func (c *appdClient) deploy(ctx context.Context, baseURL, secret string, req AppdDeployRequest) (*AppdDeployResponse, error) {
	var resp AppdDeployResponse
	if err := c.doJSON(ctx, http.MethodPost, baseURL+"/deploy", secret, req, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// deployWithBootRetry retries boot-window failures within the bounded window:
// connection-level failures (refused/reset — the container is still booting)
// and the 502/503 the gateway returns while appd itself is not yet listening.
// Genuine HTTP errors (auth, bad bundle, other 5xx) fail immediately.
func (c *appdClient) deployWithBootRetry(ctx context.Context, baseURL, secret string, req AppdDeployRequest) (*AppdDeployResponse, error) {
	deadline := time.Now().Add(c.bootRetryWindow)
	for {
		resp, err := c.deploy(ctx, baseURL, secret, req)
		if err == nil {
			return resp, nil
		}
		if !isBootRetryable(err) || time.Now().After(deadline) {
			return nil, err
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(c.bootRetryInterval):
		}
	}
}

func (c *appdClient) rollback(ctx context.Context, baseURL, secret, sha string) (*AppdDeployResponse, error) {
	var resp AppdDeployResponse
	err := c.doJSON(ctx, http.MethodPost, baseURL+"/rollback", secret, map[string]string{"sha256": sha}, &resp)
	if err != nil {
		return nil, err
	}
	return &resp, nil
}

func (c *appdClient) pushEnv(ctx context.Context, baseURL, secret string, vars map[string]string) error {
	return c.doJSON(ctx, http.MethodPost, baseURL+"/env", secret, map[string]any{"vars": vars}, nil)
}

func (c *appdClient) health(ctx context.Context, baseURL, secret string) (json.RawMessage, error) {
	return c.getRaw(ctx, baseURL+"/health", secret)
}

func (c *appdClient) logs(ctx context.Context, baseURL, secret string, q LogsQuery) (json.RawMessage, error) {
	params := url.Values{}
	if q.Lines > 0 {
		params.Set("lines", strconv.Itoa(q.Lines))
	}
	if q.Grep != "" {
		params.Set("grep", q.Grep)
	}
	if q.Since != "" {
		params.Set("since", q.Since)
	}
	if q.Stream != "" {
		params.Set("stream", q.Stream)
	}
	target := baseURL + "/logs"
	if encoded := params.Encode(); encoded != "" {
		target += "?" + encoded
	}
	return c.getRaw(ctx, target, secret)
}

func (c *appdClient) doJSON(ctx context.Context, method, target, secret string, body, out any) error {
	var reader io.Reader
	if body != nil {
		encoded, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("appd: encode request: %w", err)
		}
		reader = bytes.NewReader(encoded)
	}
	req, err := http.NewRequestWithContext(ctx, method, target, reader)
	if err != nil {
		return fmt.Errorf("appd: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("appd: %s %s: %w", method, target, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("appd: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return &AppdError{StatusCode: resp.StatusCode, Message: appdErrorMessage(payload)}
	}
	if out != nil {
		if err := json.Unmarshal(payload, out); err != nil {
			return fmt.Errorf("appd: decode response: %w", err)
		}
	}
	return nil
}

func (c *appdClient) getRaw(ctx context.Context, target, secret string) (json.RawMessage, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return nil, fmt.Errorf("appd: build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+secret)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("appd: GET %s: %w", target, err)
	}
	defer resp.Body.Close()
	payload, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, fmt.Errorf("appd: read response: %w", err)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, &AppdError{StatusCode: resp.StatusCode, Message: appdErrorMessage(payload)}
	}
	return json.RawMessage(payload), nil
}

func appdErrorMessage(payload []byte) string {
	var decoded struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(payload, &decoded); err == nil && decoded.Error != "" {
		return decoded.Error
	}
	msg := strings.TrimSpace(string(payload))
	if len(msg) > 300 {
		msg = msg[:300]
	}
	return msg
}

// isBootRetryable reports failures worth retrying while a container boots:
// transport-level errors, plus the gateway's 502/503 emitted before appd is
// listening. Other appd HTTP errors (auth, bad bundle, 4xx, other 5xx) are
// genuine and must not be retried.
func isBootRetryable(err error) bool {
	if isConnectionError(err) {
		return true
	}
	var appdErr *AppdError
	if errors.As(err, &appdErr) {
		return appdErr.StatusCode == http.StatusBadGateway ||
			appdErr.StatusCode == http.StatusServiceUnavailable
	}
	return false
}

// isConnectionError reports transport-level failures worth retrying while a
// container boots: refused/reset connections and premature closes.
func isConnectionError(err error) bool {
	if err == nil {
		return false
	}
	var appdErr *AppdError
	if errors.As(err, &appdErr) {
		return false
	}
	if errors.Is(err, syscall.ECONNREFUSED) || errors.Is(err, syscall.ECONNRESET) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	var netErr *net.OpError
	return errors.As(err, &netErr)
}
