// DO NOT EDIT — hivycore is the Hivy platform core. See doc.go.

package hivycore

import (
	"context"
	"log/slog"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// The live relay streams the bound sheet's realtime change events to browser
// clients over SSE so every app auto-refreshes with zero agent code. One
// process holds AT MOST ONE upstream connection to the platform live feed
// (lazily dialed on the first subscriber, closed a short linger after the
// last); events fan out to subscribed browsers over buffered channels. See
// live_upstream.go for the upstream connection and SSE framing.
// liveClientBuffer bounds a browser client's pending-event queue. A client
// that can't keep up overflows it and is force-resynced (see enqueue).
const liveClientBuffer = 64

// These tuning windows are package vars (not consts) purely so tests can
// shrink them; they are never reassigned in production.
var (
	// liveClientHeartbeat is how often each browser gets an SSE comment to
	// keep the connection (and any intermediary) alive; it also flushes a
	// pending forced refresh for a slow client.
	liveClientHeartbeat = 15 * time.Second
	// liveUpstreamLinger keeps the upstream connection open briefly after the
	// last browser leaves, so rapid unsubscribe/resubscribe cycles (a reload)
	// don't flap the connection.
	liveUpstreamLinger = 30 * time.Second
)

// sseFrame is one server-sent event: a type and a payload. An empty event
// name is the SSE default ("message"). data may contain newlines; each line
// is re-emitted as its own data: field so relayed payloads stay verbatim.
type sseFrame struct {
	event string
	data  string
}

// refreshFrame tells the SPA to invalidate everything sheet-related and
// refetch. It is the universal safe fallback: emitted on subscribe, after an
// upstream reconnect, and to any client that falls behind.
var refreshFrame = sseFrame{event: "refresh", data: "{}"}

// liveClient is one subscribed browser connection. Events arrive on ch;
// needRefresh is set when ch overflows so the writer emits a refresh instead
// of blocking the fan-out; closed unblocks the writer on shutdown.
type liveClient struct {
	ch          chan sseFrame
	needRefresh atomic.Bool
	closed      chan struct{}
	closeOnce   sync.Once
}

func newLiveClient() *liveClient {
	return &liveClient{
		ch:     make(chan sseFrame, liveClientBuffer),
		closed: make(chan struct{}),
	}
}

func (c *liveClient) close() { c.closeOnce.Do(func() { close(c.closed) }) }

// enqueue delivers a frame without ever blocking the fan-out. If the client's
// buffer is full it drops the frame and arms a forced refresh, so the client
// resyncs from the source of truth instead of stalling every other client.
func (c *liveClient) enqueue(f sseFrame) {
	select {
	case c.ch <- f:
	default:
		c.needRefresh.Store(true)
	}
}

// liveManager owns the single upstream connection and the set of subscribed
// browser clients. All state transitions hold mu; the upstream reader runs in
// one goroutine whose lifetime is bounded by cancel.
type liveManager struct {
	baseURL string
	secret  string
	log     *slog.Logger
	httpc   *http.Client

	mu          sync.Mutex
	clients     map[*liveClient]struct{}
	cancel      context.CancelFunc
	lingerTimer *time.Timer
	closed      bool
}

func newLiveManager(baseURL, secret string, log *slog.Logger) *liveManager {
	return &liveManager{
		baseURL: baseURL,
		secret:  secret,
		log:     log,
		// No client Timeout: the upstream stream is long-lived; cancellation
		// rides on the per-request context instead.
		httpc:   &http.Client{},
		clients: make(map[*liveClient]struct{}),
	}
}

// subscribe registers a client, (re)starting the upstream connection if this
// is the first subscriber, and queues the initial refresh so a freshly
// connected browser resyncs immediately.
func (m *liveManager) subscribe(c *liveClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	c.enqueue(refreshFrame)
	if m.closed {
		return
	}
	m.clients[c] = struct{}{}
	if m.lingerTimer != nil {
		m.lingerTimer.Stop()
		m.lingerTimer = nil
	}
	if m.cancel == nil {
		ctx, cancel := context.WithCancel(context.Background())
		m.cancel = cancel
		go m.runUpstream(ctx) // bounded by cancel; allowlisted (see goroutine-allowlist.txt)
	}
}

// unsubscribe drops a client and, when the last one leaves, arms the linger
// timer that tears the upstream connection down if no one returns.
func (m *liveManager) unsubscribe(c *liveClient) {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.clients, c)
	if len(m.clients) == 0 && m.cancel != nil && m.lingerTimer == nil {
		m.lingerTimer = time.AfterFunc(liveUpstreamLinger, m.stopIfIdle)
	}
}

// stopIfIdle closes the upstream connection if it is still unused after the
// linger window.
func (m *liveManager) stopIfIdle() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.lingerTimer = nil
	if len(m.clients) == 0 && m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
}

// broadcast fans a frame out to every subscribed client. The client set is
// snapshotted under the lock and delivered outside it so a slow writer can
// never stall the upstream reader.
func (m *liveManager) broadcast(f sseFrame) {
	m.mu.Lock()
	clients := make([]*liveClient, 0, len(m.clients))
	for c := range m.clients {
		clients = append(clients, c)
	}
	m.mu.Unlock()
	for _, c := range clients {
		c.enqueue(f)
	}
}

// Close shuts the relay down: it stops the upstream connection and unblocks
// every client writer so in-flight SSE handlers return and the HTTP server
// can shut down promptly. Wired into (*App).Run's shutdown path.
func (m *liveManager) Close() {
	m.mu.Lock()
	m.closed = true
	if m.lingerTimer != nil {
		m.lingerTimer.Stop()
		m.lingerTimer = nil
	}
	if m.cancel != nil {
		m.cancel()
		m.cancel = nil
	}
	clients := make([]*liveClient, 0, len(m.clients))
	for c := range m.clients {
		clients = append(clients, c)
	}
	m.clients = make(map[*liveClient]struct{})
	m.mu.Unlock()
	for _, c := range clients {
		c.close()
	}
}

// handleLive is GET /api/_live: the browser-facing SSE endpoint, mounted
// inside the authed /api router so the session cookie gates it. It relays
// upstream events verbatim plus synthetic refreshes, and writes per-client
// heartbeats. The handler goroutine is the writer; it returns when the client
// disconnects or the relay shuts down.
func (a *App) handleLive(w http.ResponseWriter, r *http.Request) {
	h := w.Header()
	h.Set("Content-Type", "text/event-stream")
	h.Set("Cache-Control", "no-cache")
	h.Set("Connection", "keep-alive")
	// Defeat proxy buffering so events flush in real time.
	h.Set("X-Accel-Buffering", "no")

	rc := http.NewResponseController(w)
	// SSE is long-lived; drop the server's write deadline for this connection.
	_ = rc.SetWriteDeadline(time.Time{})

	client := newLiveClient()
	a.live.subscribe(client)
	defer a.live.unsubscribe(client)

	heartbeat := time.NewTicker(liveClientHeartbeat)
	defer heartbeat.Stop()
	ctx := r.Context()

	for {
		select {
		case <-ctx.Done():
			return
		case <-client.closed:
			return
		case f := <-client.ch:
			if !writeSSE(w, f) {
				return
			}
			if client.needRefresh.CompareAndSwap(true, false) && !writeSSE(w, refreshFrame) {
				return
			}
			if rc.Flush() != nil {
				return
			}
		case <-heartbeat.C:
			if client.needRefresh.CompareAndSwap(true, false) && !writeSSE(w, refreshFrame) {
				return
			}
			if !writeComment(w) || rc.Flush() != nil {
				return
			}
		}
	}
}
