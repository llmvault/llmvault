package proxy

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
	"github.com/usehivy/hivy/internal/observe"
)

// CaptureTransport wraps an http.RoundTripper to capture response metadata
// (usage, TTFB, status) without adding latency to the response.
type CaptureTransport struct {
	Inner http.RoundTripper
}

// RoundTrip executes the HTTP request and captures response data.
// For streaming (SSE) responses, it wraps the body to parse usage as chunks
// flow through. For non-streaming responses, it reads and re-serves the body.
func (ct *CaptureTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	captured, hasCaptured := observe.CapturedDataFromContext(req.Context())

	start := time.Now()

	span := sentryobs.StartSpan(req.Context(), "llm.upstream", req.Method+" "+req.URL.Host+req.URL.Path)
	if span != nil {
		span.SetData("http.method", req.Method)
		span.SetData("http.host", req.URL.Host)
		span.SetData("http.path", req.URL.Path)
		if hasCaptured && captured.ProviderID != "" {
			span.SetData("llm.provider", captured.ProviderID)
		}
	}

	resp, err := ct.roundTrip(req, captured)
	if err != nil {
		totalMs := int(time.Since(start).Milliseconds())
		if hasCaptured {
			captured.TotalMs = totalMs
			captured.ErrorType = classifyTransportError(err)
			captured.ErrorMessage = err.Error()
		}
		logging.FromContext(req.Context()).ErrorContext(req.Context(), "proxy upstream transport error",
			"method", req.Method,
			"host", req.URL.Host,
			"path", req.URL.Path,
			"duration_ms", totalMs,
			"error", err,
		)
		sentryobs.CaptureException(req.Context(), fmt.Errorf("proxy upstream %s %s: %w", req.Method, req.URL.Host, err))
		sentryobs.FinishSpanWithError(span, err)
		return nil, err
	}
	sentryobs.FinishSpanWithError(span, nil)

	if !hasCaptured {
		return resp, nil
	}

	captured.UpstreamStatus = resp.StatusCode

	contentType := resp.Header.Get("Content-Type")
	isSSE := strings.Contains(contentType, "text/event-stream")
	captured.IsStreaming = isSSE

	if resp.StatusCode >= 400 {

		captured.TotalMs = int(time.Since(start).Milliseconds())
		captured.TTFBMs = captured.TotalMs
		captured.ErrorType = classifyHTTPError(resp.StatusCode)

		if resp.Body != nil {
			snippet := make([]byte, 512)
			n, _ := resp.Body.Read(snippet)
			if n > 0 {
				captured.ErrorMessage = string(snippet[:n])
				resp.Body = io.NopCloser(io.MultiReader(bytes.NewReader(snippet[:n]), resp.Body))
			}
		}

		if resp.StatusCode >= 500 {
			logging.FromContext(req.Context()).ErrorContext(req.Context(), "proxy upstream 5xx response",
				"method", req.Method,
				"host", req.URL.Host,
				"path", req.URL.Path,
				"status", resp.StatusCode,
				"duration_ms", captured.TotalMs,
				"snippet", captured.ErrorMessage,
			)
		}
		return resp, nil
	}

	if isSSE {
		resp.Body = &streamingCapture{
			inner:      resp.Body,
			captured:   captured,
			start:      start,
			providerID: captured.ProviderID,
		}
	} else {
		ct.captureNonStreaming(resp, captured, start)
	}

	return resp, nil
}

const maxTheGridRedirects = 5

func (ct *CaptureTransport) roundTrip(
	req *http.Request,
	captured *observe.CapturedData,
) (*http.Response, error) {
	if captured == nil || captured.ProviderID != "thegrid" {
		return ct.Inner.RoundTrip(req)
	}

	current := req
	for redirects := 0; ; redirects++ {
		resp, err := ct.Inner.RoundTrip(current)
		if err != nil || resp == nil || resp.StatusCode != http.StatusTemporaryRedirect {
			return resp, err
		}
		if redirects >= maxTheGridRedirects {
			closeResponseBody(resp)
			return nil, errors.New("thegrid redirect limit exceeded")
		}

		location, err := resp.Location()
		if err != nil {
			closeResponseBody(resp)
			return nil, fmt.Errorf("thegrid redirect location: %w", err)
		}
		if !validTheGridRedirectURL(location) {
			closeResponseBody(resp)
			return nil, errors.New("thegrid redirect destination is not allowed")
		}
		closeResponseBody(resp)

		next := current.Clone(current.Context())
		next.URL = location
		next.Host = location.Host
		next.Header = current.Header.Clone()
		if current.Body != nil {
			if current.GetBody == nil {
				return nil, errors.New("thegrid redirect cannot replay request body")
			}
			next.Body, err = current.GetBody()
			if err != nil {
				return nil, fmt.Errorf("thegrid redirect body: %w", err)
			}
			next.GetBody = current.GetBody
		}
		current = next
	}
}

func closeResponseBody(resp *http.Response) {
	if resp != nil && resp.Body != nil {
		_ = resp.Body.Close()
	}
}

func validTheGridRedirectURL(location *url.URL) bool {
	if location == nil || location.Scheme != "https" ||
		(location.Port() != "" && location.Port() != "443") {
		return false
	}
	switch strings.ToLower(location.Hostname()) {
	case "api.thegrid.ai", "synapse.thegrid.ai":
		return true
	default:
		return false
	}
}

func (ct *CaptureTransport) captureNonStreaming(resp *http.Response, captured *observe.CapturedData, start time.Time) {
	if resp.Body == nil {
		captured.TotalMs = int(time.Since(start).Milliseconds())
		captured.TTFBMs = captured.TotalMs
		return
	}

	body, err := io.ReadAll(resp.Body)
	resp.Body.Close()
	captured.TotalMs = int(time.Since(start).Milliseconds())
	captured.TTFBMs = captured.TotalMs

	if err == nil && len(body) > 0 {
		captured.Usage = toObserveUsage(ParseUsageNonStreaming(captured.ProviderID, body))
		if id := parseResponseID(body); id != "" {
			captured.GenerationID = id
		}
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
}

func classifyTransportError(err error) string {
	msg := err.Error()
	if strings.Contains(msg, "timeout") || strings.Contains(msg, "deadline") {
		return "timeout"
	}
	if strings.Contains(msg, "connection refused") || strings.Contains(msg, "no such host") {
		return "connection_error"
	}
	return "transport_error"
}

func classifyHTTPError(status int) string {
	switch {
	case status == 429:
		return "rate_limit"
	case status == 401 || status == 403:
		return "auth"
	case status >= 500:
		return "upstream_error"
	default:
		return "client_error"
	}
}
