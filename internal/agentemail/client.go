// Package agentemail implements the small Resend API surface required for
// agent inboxes. Keeping it here makes the webhook body retrieval and the MCP
// send tool share one typed, server-side-only client.
package agentemail

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

const maxAPIResponseBytes = 8 << 20

type Client struct {
	apiKey  string
	baseURL string
	http    *http.Client
}

func NewClient(apiKey, baseURL string) *Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		baseURL = "https://api.resend.com"
	}
	return &Client{apiKey: strings.TrimSpace(apiKey), baseURL: baseURL, http: &http.Client{Timeout: 30 * time.Second}}
}

func (c *Client) Configured() bool { return c != nil && c.apiKey != "" }

type ReceivedEmail struct {
	ID        string            `json:"id"`
	From      string            `json:"from"`
	To        []string          `json:"to"`
	CC        []string          `json:"cc"`
	BCC       []string          `json:"bcc"`
	Subject   string            `json:"subject"`
	Text      *string           `json:"text"`
	HTML      *string           `json:"html"`
	Headers   map[string]string `json:"headers"`
	MessageID string            `json:"message_id"`
	CreatedAt time.Time         `json:"created_at"`
}

func (c *Client) GetReceived(ctx context.Context, emailID string) (ReceivedEmail, error) {
	var result ReceivedEmail
	if !c.Configured() {
		return result, fmt.Errorf("resend API key is not configured")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/emails/receiving/"+emailID, nil)
	if err != nil {
		return result, fmt.Errorf("build Resend receive request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	if err := c.doJSON(req, &result); err != nil {
		return result, fmt.Errorf("retrieve Resend received email: %w", err)
	}
	return result, nil
}

type SendRequest struct {
	From    string            `json:"from"`
	To      []string          `json:"to"`
	CC      []string          `json:"cc,omitempty"`
	BCC     []string          `json:"bcc,omitempty"`
	Subject string            `json:"subject"`
	Text    string            `json:"text,omitempty"`
	HTML    string            `json:"html,omitempty"`
	Headers map[string]string `json:"headers,omitempty"`
	ReplyTo string            `json:"reply_to,omitempty"`
}

type SendResponse struct {
	ID string `json:"id"`
}

func (c *Client) Send(ctx context.Context, input SendRequest, idempotencyKey string) (SendResponse, error) {
	var result SendResponse
	if !c.Configured() {
		return result, fmt.Errorf("resend API key is not configured")
	}
	body, err := json.Marshal(input)
	if err != nil {
		return result, fmt.Errorf("encode Resend email: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/emails", bytes.NewReader(body))
	if err != nil {
		return result, fmt.Errorf("build Resend send request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+c.apiKey)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idempotencyKey)
	if err := c.doJSON(req, &result); err != nil {
		return result, fmt.Errorf("send Resend email: %w", err)
	}
	return result, nil
}

func (c *Client) doJSON(req *http.Request, out any) error {
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(io.LimitReader(resp.Body, maxAPIResponseBytes))
	if err != nil {
		return err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	if err := json.Unmarshal(body, out); err != nil {
		return err
	}
	return nil
}
