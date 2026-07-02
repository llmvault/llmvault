package quiver

import (
	"fmt"
	"net/http"
	"strings"
	"time"
)

// DefaultModel is Quiver's recommended default SVG model.
const DefaultModel = "arrow-1.1"

// Client talks to the Quiver AI SVG generation API.
type Client struct {
	httpClient *http.Client
	timeout    time.Duration
}

// NewClient builds a Quiver client. httpClient may be nil (http.DefaultClient
// is used) and timeout <= 0 disables the per-call deadline.
func NewClient(httpClient *http.Client, timeout time.Duration) *Client {
	return &Client{httpClient: httpClient, timeout: timeout}
}

// Reference is an input image for a Quiver request. Exactly one of URL or
// Base64Data should be set; Base64Data takes precedence when both are present.
type Reference struct {
	URL        string
	Base64Data string
}

// GenerateRequest describes a text-to-svg generation call
// (POST /v1/svgs/generations).
type GenerateRequest struct {
	APIKey       string
	BaseURL      string
	AuthScheme   string
	Model        string
	Prompt       string
	Instructions string
	// N is the number of SVGs to generate (1-16). Values <= 1 omit the field
	// so Quiver defaults to a single output.
	N          int
	References []Reference
}

// VectorizeRequest describes an image-to-svg vectorization call
// (POST /v1/svgs/vectorizations).
type VectorizeRequest struct {
	APIKey     string
	BaseURL    string
	AuthScheme string
	Model      string
	Image      Reference
}

// SVG is a single generated vector image.
type SVG struct {
	Markup   string
	MimeType string
}

// Result is a decoded Quiver response plus billing metadata.
type Result struct {
	SVGs       []SVG
	ResponseID string
	Credits    int
	RequestID  string
}

// StatusError is returned for non-2xx Quiver responses.
type StatusError struct {
	StatusCode int
	Code       string
	Message    string
	RequestID  string
	RetryAfter string
	Body       string
}

func (e *StatusError) Error() string {
	parts := []string{fmt.Sprintf("quiver svg generation failed with status %d", e.StatusCode)}
	if e.Code != "" {
		parts = append(parts, "code "+e.Code)
	}
	if e.Message != "" {
		parts = append(parts, e.Message)
	} else if e.Body != "" {
		parts = append(parts, e.Body)
	}
	if e.RequestID != "" {
		parts = append(parts, "request_id "+e.RequestID)
	}
	return strings.Join(parts, ": ")
}
