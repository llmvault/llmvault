package proxy

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/usehivy/hivy/internal/observe"
)

// sseUpstream writes raw byte chunks (so the test controls exact SSE framing),
// flushing between them to keep the response streaming.
func sseUpstream(t *testing.T, chunks ...string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher, _ := w.(http.Flusher)
		for _, c := range chunks {
			fmt.Fprint(w, c)
			if flusher != nil {
				flusher.Flush()
			}
		}
	}))
}

func runCapture(t *testing.T, upstream *httptest.Server, providerID string) *observe.CapturedData {
	t.Helper()
	captured := &observe.CapturedData{ProviderID: providerID}
	ctx := observe.WithCapturedData(context.Background(), captured)
	req, _ := http.NewRequestWithContext(ctx, "POST", upstream.URL, nil)

	ct := &CaptureTransport{Inner: http.DefaultTransport}
	resp, err := ct.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatalf("read body: %v", err)
	}
	resp.Body.Close()
	return captured
}

// CRLF-framed (\r\n) SSE streams must have their usage parsed; byte-offset
// accounting must not drift on the stripped \r.
func TestStreamingCapture_CRLF(t *testing.T) {
	upstream := sseUpstream(t,
		"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\r\n\r\n",
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":200,\"completion_tokens\":100}}\r\n\r\n",
		"data: [DONE]\r\n\r\n",
	)
	defer upstream.Close()

	captured := runCapture(t, upstream, "openai")

	if captured.Usage.InputTokens != 200 {
		t.Errorf("InputTokens = %d, want 200 (CRLF framing dropped usage)", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 100 {
		t.Errorf("OutputTokens = %d, want 100", captured.Usage.OutputTokens)
	}
}

// A final usage event with no trailing newline (provider closes right after the
// JSON) must still be parsed on flush.
func TestStreamingCapture_NoTrailingNewline(t *testing.T) {
	upstream := sseUpstream(t,
		"data: {\"choices\":[{\"delta\":{\"content\":\"hi\"}}]}\n\n",
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":42,\"completion_tokens\":7}}",
	)
	defer upstream.Close()

	captured := runCapture(t, upstream, "openai")

	if captured.Usage.InputTokens != 42 {
		t.Errorf("InputTokens = %d, want 42 (final unterminated usage dropped)", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 7 {
		t.Errorf("OutputTokens = %d, want 7", captured.Usage.OutputTokens)
	}
}

// Usage after a >64KB data line must still parse; bufio.Scanner's 64KB
// MaxScanTokenSize used to silently drop everything past it.
func TestStreamingCapture_LargeLine(t *testing.T) {
	big := strings.Repeat("x", 200*1024) // 200KB, well past bufio's 64KB cap
	upstream := sseUpstream(t,
		fmt.Sprintf("data: {\"choices\":[{\"delta\":{\"content\":\"%s\"}}]}\n\n", big),
		"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":999,\"completion_tokens\":111}}\n\n",
		"data: [DONE]\n\n",
	)
	defer upstream.Close()

	captured := runCapture(t, upstream, "openai")

	if captured.Usage.InputTokens != 999 {
		t.Errorf("InputTokens = %d, want 999 (usage after >64KB line dropped)", captured.Usage.InputTokens)
	}
	if captured.Usage.OutputTokens != 111 {
		t.Errorf("OutputTokens = %d, want 111", captured.Usage.OutputTokens)
	}
}

// A usage event split across multiple Read() calls must parse correctly under
// the partial-line buffering.
func TestStreamingCapture_SplitAcrossReads(t *testing.T) {
	full := "data: {\"choices\":[],\"usage\":{\"prompt_tokens\":55,\"completion_tokens\":66}}\n\n"
	mid := len(full) / 2
	upstream := sseUpstream(t, full[:mid], full[mid:], "data: [DONE]\n\n")
	defer upstream.Close()

	captured := runCapture(t, upstream, "openai")

	if captured.Usage.InputTokens != 55 || captured.Usage.OutputTokens != 66 {
		t.Errorf("usage = %d/%d, want 55/66 (split-line accounting broken)",
			captured.Usage.InputTokens, captured.Usage.OutputTokens)
	}
}

// An un-terminated, ever-growing line must not accumulate past the hard cap.
func TestStreamingCapture_BufferCapped(t *testing.T) {
	sc := &streamingCapture{captured: &observe.CapturedData{}}
	sc.buf.WriteString(strings.Repeat("y", maxStreamBufLen+1024))
	sc.tryParseEvents(false)
	if sc.buf.Len() > maxStreamBufLen {
		t.Errorf("buffer len %d exceeds cap %d; unbounded growth not prevented", sc.buf.Len(), maxStreamBufLen)
	}
}
