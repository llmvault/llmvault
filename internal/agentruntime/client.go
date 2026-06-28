package agentruntime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type Client struct {
	baseURL string
	apiKey  string
	http    *http.Client
	// stream is a dedicated client for long-lived SSE bodies: no overall Timeout
	// (Go's covers the whole body read and would cut long turns mid-answer); only
	// dial/header bounded, liveness from the request context.
	stream *http.Client
}

const defaultHTTPTimeout = 2 * time.Minute

type SyncResponse struct {
	Applied          int      `json:"applied"`
	Deleted          int      `json:"deleted"`
	RestartTriggered bool     `json:"restart_triggered"`
	Errors           []string `json:"errors,omitempty"`
}

type ConfigUpdateRequest struct {
	RuntimeSecret string            `json:"runtime_secret,omitempty"`
	RuntimeEnv    map[string]string `json:"runtime_env,omitempty"`
	Definition    *AgentDefinition  `json:"definition"`
	Workspace     *WorkspaceConfig  `json:"workspace,omitempty"`
}

type HTTPMessageRequest struct {
	Text            string       `json:"text"`
	SessionID       string       `json:"-"`
	SessionContext  []string     `json:"session_context,omitempty"`
	User            string       `json:"user,omitempty"`
	UserDisplayName string       `json:"user_display_name,omitempty"`
	ModelDefinition *ModelConfig `json:"model_definition,omitempty"`
	Attachments     []any        `json:"attachments,omitempty"`
}

type HTTPMessageResponse struct {
	SessionID         string `json:"session_id"`
	StreamID          string `json:"stream_id"`
	StreamURL         string `json:"stream_url"`
	ResponseStreamID  string `json:"response_stream_id"`
	ResponseStreamURL string `json:"response_stream_url"`
	TraceID           string `json:"trace_id"`
	TurnID            string `json:"turn_id"`
}

type InterruptSessionResponse struct {
	Interrupted bool   `json:"interrupted"`
	Status      string `json:"status"`
}

type DrainStatusResponse struct {
	Status                  string     `json:"status"`
	Draining                bool       `json:"draining"`
	Complete                bool       `json:"complete"`
	ActiveTurns             int        `json:"active_turns"`
	PendingAcceptedMessages int        `json:"pending_accepted_messages"`
	PendingOutboxEvents     int64      `json:"pending_outbox_events"`
	StartedAt               *time.Time `json:"started_at,omitempty"`
	CompletedAt             *time.Time `json:"completed_at,omitempty"`
}

type HTTPStatusError struct {
	Op         string
	StatusCode int
	Status     string
	Body       string
}

func (e *HTTPStatusError) Error() string {
	return fmt.Sprintf("%s: %s: %s", e.Op, e.Status, e.Body)
}

func IsRuntimeDrainingError(err error) bool {
	var statusErr *HTTPStatusError
	if !errors.As(err, &statusErr) {
		return false
	}
	return statusErr.StatusCode == http.StatusConflict && strings.Contains(strings.ToLower(statusErr.Body), "drain")
}

func NewClient(baseURL, apiKey string) *Client {
	return NewClientWithTimeout(baseURL, apiKey, defaultHTTPTimeout)
}

func NewClientWithTimeout(baseURL, apiKey string, timeout time.Duration) *Client {
	if timeout <= 0 {
		timeout = defaultHTTPTimeout
	}
	return &Client{
		baseURL: strings.TrimRight(baseURL, "/"),
		apiKey:  apiKey,
		http:    &http.Client{Timeout: timeout},
		stream:  newStreamingHTTPClient(),
	}
}

// newStreamingHTTPClient builds the SSE client for live runtime streams (Timeout
// 0; only dial/TLS/header bounded), per the `stream` field contract above.
func newStreamingHTTPClient() *http.Client {
	transport := &http.Transport{
		Proxy: http.ProxyFromEnvironment,
		DialContext: (&net.Dialer{
			Timeout:   10 * time.Second,
			KeepAlive: 30 * time.Second,
		}).DialContext,
		TLSHandshakeTimeout:   10 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// Never compress an SSE body; we also send Accept-Encoding: identity.
		DisableCompression: true,
		ForceAttemptHTTP2:  true,
	}
	return &http.Client{Transport: transport, Timeout: 0}
}

func (c *Client) Healthz(ctx context.Context) error {
	resp, err := c.doRuntimeRequest(ctx, http.MethodGet, "/healthz", nil, false)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("healthz: %s", resp.Status)
	}
	return nil
}

func (c *Client) Readyz(ctx context.Context) error {
	return c.doVoid(ctx, http.MethodGet, "/readyz", nil)
}

func (c *Client) GetConfig(ctx context.Context) (*AgentDefinition, error) {
	resp, err := c.do(ctx, http.MethodGet, "/config", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get config: %s: %s", resp.Status, body)
	}
	var out AgentDefinition
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode config: %w", err)
	}
	return &out, nil
}

func (c *Client) PutRuntimeConfig(ctx context.Context, body ConfigUpdateRequest) (*SyncResponse, error) {
	if body.RuntimeEnv == nil {
		body.RuntimeEnv = map[string]string{}
	}
	resp, err := c.doGzip(ctx, http.MethodPut, "/config", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("put config: %s: %s", resp.Status, body)
	}
	var out struct {
		EnvKeyCount int `json:"env_key_count"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode config response: %w", err)
	}
	return &SyncResponse{Applied: 1, RestartTriggered: true}, nil
}

func (c *Client) StartDrain(ctx context.Context) (*DrainStatusResponse, error) {
	return c.drain(ctx, http.MethodPost, "/control/drain", "start drain")
}

func (c *Client) DrainStatus(ctx context.Context) (*DrainStatusResponse, error) {
	return c.drain(ctx, http.MethodGet, "/control/drain", "get drain status")
}

func (c *Client) CancelDrain(ctx context.Context) (*DrainStatusResponse, error) {
	return c.drain(ctx, http.MethodPost, "/control/drain/cancel", "cancel drain")
}

func (c *Client) drain(ctx context.Context, method, path, op string) (*DrainStatusResponse, error) {
	resp, err := c.do(ctx, method, path, nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, &HTTPStatusError{Op: op, StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(raw))}
	}
	var out DrainStatusResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("%s: decode response: %w", op, err)
	}
	return &out, nil
}

func (c *Client) PostHTTPMessage(ctx context.Context, body HTTPMessageRequest) (*HTTPMessageResponse, error) {
	sessionID := strings.TrimSpace(body.SessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("post session message: session_id is required")
	}
	resp, err := c.do(ctx, http.MethodPost, "/sessions/"+url.PathEscape(sessionID)+"/messages", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, &HTTPStatusError{Op: "post session message", StatusCode: resp.StatusCode, Status: resp.Status, Body: strings.TrimSpace(string(raw))}
	}
	var out HTTPMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode session message response: %w", err)
	}
	return &out, nil
}

func (c *Client) InterruptSession(ctx context.Context, sessionID string) (*InterruptSessionResponse, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("interrupt session: session_id is required")
	}
	resp, err := c.do(ctx, http.MethodPost, "/sessions/"+url.PathEscape(sessionID)+"/interrupt", nil)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("interrupt session: %s: %s", resp.Status, raw)
	}
	var out InterruptSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode interrupt session response: %w", err)
	}
	return &out, nil
}
