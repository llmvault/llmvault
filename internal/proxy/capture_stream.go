package proxy

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/usehivy/hivy/internal/observe"
)

// maxStreamBufLen caps the partial-line buffer so a malformed provider stream
// (a never-terminated line) cannot grow it unboundedly; real SSE usage events
// never approach this size. An un-terminated line past the cap is discarded.
const maxStreamBufLen = 1 << 20 // 1 MiB

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
		// Terminal error: flush any unterminated trailing line so final usage isn't lost.
		sc.tryParseEvents(true)
	}

	return n, err
}

func (sc *streamingCapture) Close() error {
	sc.captured.TotalMs = int(time.Since(sc.start).Milliseconds())
	sc.tryParseEvents(true)
	return sc.inner.Close()
}

// tryParseEvents extracts usage from complete `data:` SSE lines, scanning raw
// `\n` offsets (exact reset accounting) and trimming `\r` so CRLF parses.
// Unterminated lines stay buffered; flush==true parses a final trailing one.
func (sc *streamingCapture) tryParseEvents(flush bool) {
	for {
		data := sc.buf.Bytes()
		idx := bytes.IndexByte(data, '\n')
		if idx < 0 {
			// No complete line. On flush, parse the remainder as a final line.
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

	// Hard cap: discard an un-terminated line past the limit so memory is bounded.
	if !flush && sc.buf.Len() > maxStreamBufLen {
		sc.buf.Reset()
	}
}

// parseLine extracts usage from a single (newline/CR-stripped) SSE line,
// ignoring non-`data:` lines and `[DONE]`.
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
		sc.captured.Usage.InputTokens = u.InputTokens
		sc.captured.Usage.OutputTokens = u.OutputTokens
		sc.captured.Usage.CachedTokens = u.CachedTokens
		sc.captured.Usage.ReasoningTokens = u.ReasoningTokens
	}
	if u.ProviderCostUSD > 0 {
		sc.captured.Usage.ProviderCostUSD = u.ProviderCostUSD
	}
	if sc.captured.GenerationID == "" {
		if id := parseResponseID(payload); id != "" {
			sc.captured.GenerationID = id
		}
	}
}

// parseResponseID extracts the top-level "id" string from a provider response
// (non-streaming body or a single SSE chunk). OpenRouter returns its generation
// id here, used for post-hoc usage reconciliation.
func parseResponseID(body []byte) string {
	var resp struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return ""
	}
	return resp.ID
}

func toObserveUsage(u UsageData) observe.UsageData {
	return observe.UsageData{
		InputTokens:     u.InputTokens,
		OutputTokens:    u.OutputTokens,
		CachedTokens:    u.CachedTokens,
		ReasoningTokens: u.ReasoningTokens,
		ProviderCostUSD: u.ProviderCostUSD,
	}
}
