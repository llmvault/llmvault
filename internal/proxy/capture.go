package proxy

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/usehivy/hivy/internal/logging"
	sentryobs "github.com/usehivy/hivy/internal/observability/sentry"
	"github.com/usehivy/hivy/internal/observe"
)

// maxStreamBufLen caps the partial-line buffer so a malformed provider stream
// (e.g. a never-terminated line) cannot grow it unboundedly. SSE usage events
// are small JSON objects; a real `data:` line never approaches this size. When
// a single un-terminated line exceeds the cap we discard the accumulated prefix
// rather than retain memory forever.
const maxStreamBufLen = 1 << 20 // 1 MiB

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

	resp, err := ct.Inner.RoundTrip(req)
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
	}

	resp.Body = io.NopCloser(bytes.NewReader(body))
}

// streamingCapture wraps an SSE response body, parsing usage from chunks
// as they flow through without adding latency.
type streamingCapture struct {
	inner      io.ReadCloser
	captured   *observe.CapturedData
	start      time.Time
	providerID string
	gotFirst   bool
	buf        bytes.Buffer // accumulates partial lines
}

func (sc *streamingCapture) Read(p []byte) (int, error) {
	n, err := sc.inner.Read(p)

	if n > 0 && !sc.gotFirst {
		sc.gotFirst = true
		sc.captured.TTFBMs = int(time.Since(sc.start).Milliseconds())
	}

	if n > 0 {
		sc.buf.Write(p[:n])
		sc.tryParseEvents(false)
	}

	if err != nil {
		sc.captured.TotalMs = int(time.Since(sc.start).Milliseconds())
		// EOF (or any terminal error) means no more bytes will arrive: flush any
		// trailing line that was not newline-terminated so a final usage event
		// is not dropped.
		sc.tryParseEvents(true)
	}

	return n, err
}

func (sc *streamingCapture) Close() error {
	sc.captured.TotalMs = int(time.Since(sc.start).Milliseconds())
	sc.tryParseEvents(true)
	return sc.inner.Close()
}

// tryParseEvents parses complete SSE lines out of the accumulated buffer,
// extracting usage from `data:` events. It scans on raw `\n` byte offsets so
// the buffer-reset accounting is exact regardless of line content, and trims a
// trailing `\r` so CRLF-framed provider streams (`\r\n`) parse correctly. Lines
// without a terminating `\n` are kept buffered for the next read; only the
// unparsed tail is retained.
//
// flush==true (called from Close) parses any final line that arrived without a
// trailing newline — e.g. a usage event on the last line of a stream that the
// provider did not terminate with `\n`.
func (sc *streamingCapture) tryParseEvents(flush bool) {
	for {
		data := sc.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			// No complete line. On flush, parse whatever remains as a final
			// line; otherwise keep it buffered (capped below).
			if flush && len(data) > 0 {
				sc.parseLine(data)
				sc.buf.Reset()
			}
			break
		}

		line := data[:idx]
		// Strip the trailing CR of a CRLF terminator.
		if n := len(line); n > 0 && line[n-1] == '\r' {
			line = line[:n-1]
		}
		sc.parseLine(line)

		// Drop the consumed line (including the `\n`) from the buffer.
		remaining := append([]byte(nil), data[idx+1:]...)
		sc.buf.Reset()
		sc.buf.Write(remaining)
	}

	// Hard cap: an un-terminated line that grows past the limit is malformed;
	// discard it so memory cannot grow without bound.
	if !flush && sc.buf.Len() > maxStreamBufLen {
		sc.buf.Reset()
	}
}

// parseLine extracts usage from a single (already newline- and CR-stripped) SSE
// line. Non-`data:` lines and the `[DONE]` sentinel are ignored.
func (sc *streamingCapture) parseLine(line []byte) {
	const prefix = "data:"
	if !bytes.HasPrefix(line, []byte(prefix)) {
		return
	}
	// Per the SSE grammar a single optional space after the colon is stripped.
	payload := bytes.TrimPrefix(line[len(prefix):], []byte(" "))
	if string(payload) == "[DONE]" || len(payload) == 0 {
		return
	}
	u := ParseStreamingChunk(sc.providerID, payload)
	if u.InputTokens > 0 || u.OutputTokens > 0 {
		sc.captured.Usage = toObserveUsage(u)
	}
}

// toObserveUsage converts proxy.UsageData to observe.UsageData.
func toObserveUsage(u UsageData) observe.UsageData {
	return observe.UsageData{
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
		CachedTokens:    u.CachedTokens,
		ReasoningTokens: u.ReasoningTokens,
	}
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
