package employeeruntime

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
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
	ReposCloned      int      `json:"repos_cloned"`
	RestartTriggered bool     `json:"restart_triggered"`
	Errors           []string `json:"errors,omitempty"`
}

type ConfigUpdateRequest struct {
	RuntimeSecret string            `json:"runtime_secret,omitempty"`
	RuntimeEnv    map[string]string `json:"runtime_env,omitempty"`
	Definition    *AgentDefinition  `json:"definition"`
	Schedules     []RuntimeSchedule `json:"schedules,omitempty"`
}

type RuntimeSchedule struct {
	ID                    string     `json:"id"`
	Description           string     `json:"description"`
	Channel               string     `json:"channel"`
	TaskPrompt            string     `json:"task_prompt"`
	CronExpression        *string    `json:"cron_expression"`
	IntervalSeconds       *uint64    `json:"interval_seconds"`
	RepeatCount           *uint32    `json:"repeat_count"`
	RepeatCompleted       uint32     `json:"repeat_completed"`
	State                 string     `json:"state"`
	Source                string     `json:"source"`
	NextRunAt             time.Time  `json:"next_run_at"`
	LastRunAt             *time.Time `json:"last_run_at"`
	LastStatus            *string    `json:"last_status"`
	LastError             *string    `json:"last_error"`
	DelegatedSessionID    *string    `json:"delegated_session_id"`
	SessionContinuationID *string    `json:"session_continuation_id"`
	AgentName             *string    `json:"agent_name,omitempty"`
	LastResult            *string    `json:"last_result,omitempty"`
	DelegateStreamID      *string    `json:"delegate_stream_id,omitempty"`
	CreatedAt             time.Time  `json:"created_at"`
	CreatedBySession      string     `json:"created_by_session"`
}

type HTTPMessageRequest struct {
	Text            string         `json:"text"`
	ConversationID  string         `json:"conversation_id,omitempty"`
	User            string         `json:"user,omitempty"`
	UserDisplayName string         `json:"user_display_name,omitempty"`
	Attachments     []any          `json:"attachments,omitempty"`
	DynamicContext  []string       `json:"dynamic_context,omitempty"`
	Raw             map[string]any `json:"raw,omitempty"`
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
	if os.Getenv("HIVY_DEBUG_RUNTIME_CONFIG_PAYLOAD") == "true" {
		// Redact secrets before logging so a debug flag can't exfiltrate credentials.
		payload, err := json.Marshal(redactConfigUpdateRequest(body))
		if err != nil {
			slog.WarnContext(ctx, "runtime config debug payload marshal failed", "error", err)
		} else {
			slog.InfoContext(ctx, "runtime config debug payload", "base_url", c.baseURL, "payload", string(payload))
		}
	}
	if path := strings.TrimSpace(os.Getenv("HIVY_DEBUG_RUNTIME_CONFIG_PAYLOAD_FILE")); path != "" {
		payload, err := json.MarshalIndent(redactConfigUpdateRequest(body), "", "  ")
		if err != nil {
			slog.WarnContext(ctx, "runtime config debug payload file marshal failed", "error", err)
		} else if err := os.WriteFile(path, payload, 0o600); err != nil {
			slog.WarnContext(ctx, "runtime config debug payload file write failed", "path", path, "error", err)
		} else {
			slog.InfoContext(ctx, "runtime config debug payload written", "path", path, "bytes", len(payload), "base_url", c.baseURL)
		}
	}
	resp, err := c.do(ctx, http.MethodPut, "/config", body)
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

func (c *Client) PostHTTPMessage(ctx context.Context, body HTTPMessageRequest) (*HTTPMessageResponse, error) {
	resp, err := c.do(ctx, http.MethodPost, "/gateway/http/messages", body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		raw, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("post http message: %s: %s", resp.Status, raw)
	}
	var out HTTPMessageResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode http message response: %w", err)
	}
	return &out, nil
}
