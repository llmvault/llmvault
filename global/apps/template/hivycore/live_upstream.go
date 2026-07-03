// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strings"
	"time"
)

// Upstream connection to the platform live feed: a single SSE stream at
// {HIVY_APP_API_URL}/v1/live, authenticated with the app secret. It
// auto-reconnects with capped exponential backoff + jitter, and after any
// successful reconnect broadcasts a synthetic refresh so every browser
// resyncs the state it may have missed while the stream was down.
const (
	liveUpstreamPath       = "/v1/live"
	liveUpstreamMaxBackoff = 30 * time.Second
	// liveUpstreamErrBodyLimit caps how much of a non-2xx upstream body we
	// drain before retrying.
	liveUpstreamErrBodyLimit = 4 << 10
)

// liveUpstreamInitialBackoff is the first reconnect delay (doubled, capped,
// and jittered thereafter). A package var so tests can shrink it; never
// reassigned in production.
var liveUpstreamInitialBackoff = 500 * time.Millisecond

// runUpstream keeps a single upstream connection alive for as long as ctx is
// live, reconnecting on any drop. It is the one long-lived goroutine the relay
// spawns; a deferred recover keeps a transport panic from crashing the app.
func (m *liveManager) runUpstream(ctx context.Context) {
	defer func() {
		if v := recover(); v != nil {
			m.log.Error("live upstream panic", "panic", v)
		}
	}()

	backoff := liveUpstreamInitialBackoff
	connectedOnce := false
	for {
		if ctx.Err() != nil {
			return
		}
		// The second and later successful connections are reconnects: signal a
		// full resync to every client once the stream is back.
		connected, err := m.streamOnce(ctx, connectedOnce)
		if ctx.Err() != nil {
			return
		}
		if connected {
			connectedOnce = true
			backoff = liveUpstreamInitialBackoff
		} else if err != nil {
			m.log.Debug("live upstream dial failed", "error", err.Error())
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(jitterBackoff(backoff)):
		}
		backoff = min(backoff*2, liveUpstreamMaxBackoff)
	}
}

// streamOnce dials the upstream feed and relays frames until the stream ends
// or ctx is cancelled. It reports whether the connection was established
// (regardless of how it ended) so the caller can reset backoff and decide
// whether the NEXT connect is a reconnect. When isReconnect is set and the
// connection succeeds, it broadcasts a refresh before relaying.
func (m *liveManager) streamOnce(ctx context.Context, isReconnect bool) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, m.baseURL+liveUpstreamPath, nil)
	if err != nil {
		return false, err
	}
	req.Header.Set("Authorization", "Bearer "+m.secret)
	req.Header.Set("Accept", "text/event-stream")

	resp, err := m.httpc.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, liveUpstreamErrBodyLimit))
		return false, fmt.Errorf("hivycore: live upstream status %d", resp.StatusCode)
	}
	if isReconnect {
		m.broadcast(refreshFrame)
	}
	return true, m.readSSE(ctx, resp.Body)
}

// readSSE parses the upstream SSE stream and broadcasts each complete event
// verbatim. Comment lines (heartbeats, starting with ':') are ignored; id and
// retry fields are dropped. Blocking reads unblock when ctx cancellation
// closes the connection.
func (m *liveManager) readSSE(ctx context.Context, body io.Reader) error {
	reader := bufio.NewReader(body)
	var event string
	var data []string
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(data) > 0 {
				m.broadcast(sseFrame{event: event, data: strings.Join(data, "\n")})
			}
			event = ""
			data = data[:0]
			if ctx.Err() != nil {
				return ctx.Err()
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue // comment / heartbeat
		}
		field, value := splitSSEField(line)
		switch field {
		case "event":
			event = value
		case "data":
			data = append(data, value)
		}
	}
}

// splitSSEField splits one SSE line into its field name and value, stripping a
// single optional space after the colon (per the SSE spec). A line with no
// colon is a field name with an empty value.
func splitSSEField(line string) (string, string) {
	idx := strings.IndexByte(line, ':')
	if idx < 0 {
		return line, ""
	}
	field := line[:idx]
	value := line[idx+1:]
	value = strings.TrimPrefix(value, " ")
	return field, value
}

// writeSSE writes one event to a browser client. Multi-line data is re-emitted
// as multiple data: fields so the relayed payload is byte-for-byte faithful.
// It reports success so the writer can bail on a broken connection.
func writeSSE(w io.Writer, f sseFrame) bool {
	var b strings.Builder
	if f.event != "" {
		b.WriteString("event: ")
		b.WriteString(f.event)
		b.WriteByte('\n')
	}
	for _, line := range strings.Split(f.data, "\n") {
		b.WriteString("data: ")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	b.WriteByte('\n')
	_, err := io.WriteString(w, b.String())
	return err == nil
}

// writeComment sends an SSE heartbeat comment.
func writeComment(w io.Writer) bool {
	_, err := io.WriteString(w, ": ping\n\n")
	return err == nil
}

// jitterBackoff applies equal jitter to a backoff duration: half the base plus
// a random point in the other half, so reconnecting clients spread out.
func jitterBackoff(base time.Duration) time.Duration {
	if base <= 0 {
		return 0
	}
	half := base / 2
	return half + time.Duration(rand.Int63n(int64(half)+1))
}
