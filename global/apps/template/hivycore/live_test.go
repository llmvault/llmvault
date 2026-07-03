package hivycore

import (
	"bufio"
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeUpstream is a scriptable stand-in for the platform live feed. It counts
// connections (to prove the relay holds at most one), pushes scripted events
// to whichever connection is current, and can drop the connection on demand to
// exercise reconnect + refresh.
type fakeUpstream struct {
	server *httptest.Server

	mu         sync.Mutex
	active     int
	peak       int
	total      int
	lastBearer string

	events chan sseFrame
	drop   chan struct{}
}

func newFakeUpstream(t *testing.T) *fakeUpstream {
	t.Helper()
	u := &fakeUpstream{
		events: make(chan sseFrame, 16),
		drop:   make(chan struct{}),
	}
	u.server = httptest.NewServer(http.HandlerFunc(u.serve))
	t.Cleanup(u.server.Close)
	return u
}

func (u *fakeUpstream) serve(w http.ResponseWriter, r *http.Request) {
	u.mu.Lock()
	u.active++
	u.total++
	if u.active > u.peak {
		u.peak = u.active
	}
	u.lastBearer = r.Header.Get("Authorization")
	u.mu.Unlock()
	defer func() {
		u.mu.Lock()
		u.active--
		u.mu.Unlock()
	}()

	w.Header().Set("Content-Type", "text/event-stream")
	w.WriteHeader(http.StatusOK)
	rc := http.NewResponseController(w)
	_ = rc.Flush()

	heartbeat := time.NewTicker(20 * time.Millisecond)
	defer heartbeat.Stop()
	for {
		select {
		case <-r.Context().Done():
			return
		case <-u.drop:
			return
		case f := <-u.events:
			_ = writeSSE(w, f)
			if rc.Flush() != nil {
				return
			}
		case <-heartbeat.C:
			_, _ = w.Write([]byte(": hb\n\n"))
			if rc.Flush() != nil {
				return
			}
		}
	}
}

func (u *fakeUpstream) push(f sseFrame) { u.events <- f }
func (u *fakeUpstream) dropConnection() { u.drop <- struct{}{} }
func (u *fakeUpstream) snapshot() (int, int) {
	u.mu.Lock()
	defer u.mu.Unlock()
	return u.active, u.total
}
func (u *fakeUpstream) peakConns() int { u.mu.Lock(); defer u.mu.Unlock(); return u.peak }

// newLiveTestApp builds a real hivycore App whose sheets/live upstream point
// at the fake feed, fronted by a real HTTP server (SSE needs real streaming,
// not httptest.ResponseRecorder).
func newLiveTestApp(t *testing.T, upstreamURL string) (*App, *httptest.Server, *rsa.PrivateKey) {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("rsa key: %v", err)
	}
	der, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		t.Fatalf("marshal pub: %v", err)
	}
	pub, err := parseRSAPublicKey(string(pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der})))
	if err != nil {
		t.Fatalf("parse pub: %v", err)
	}
	publicDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(publicDir, "index.html"), []byte("<!doctype html><title>t</title>"), 0o644); err != nil {
		t.Fatalf("index.html: %v", err)
	}
	app, err := newApp(&Config{
		AppID:         testAppID,
		AppSecret:     "hvapp_livesecret",
		APIBaseURL:    upstreamURL,
		AuthPublicKey: pub,
		LaunchURL:     testLaunchURL,
		SessionSecret: "test-session-secret",
		SheetID:       "9d1c2b3a-4444-4f4f-bbbb-dddddddddddd",
		Port:          "8080",
		PublicDir:     publicDir,
	})
	if err != nil {
		t.Fatalf("newApp: %v", err)
	}
	server := httptest.NewServer(app.Handler())
	t.Cleanup(server.Close)
	t.Cleanup(app.live.Close)
	return app, server, key
}

func liveSessionCookie(t *testing.T, app *App, key *rsa.PrivateKey) *http.Cookie {
	t.Helper()
	cookie := sessionCookieFrom(t, callbackRequest(app, signLaunchToken(t, key, tokenOverrides{})))
	if cookie == nil {
		t.Fatal("no session cookie")
	}
	return cookie
}

// liveClientConn is a browser connected to /api/_live, reading SSE frames.
type liveClientConn struct {
	resp   *http.Response
	frames chan sseFrame
	cancel context.CancelFunc
}

// dialLive opens an authenticated /api/_live stream and reads frames into a
// channel. slow=true makes the reader pause after the first frame so the
// server-side buffer overflows (exercising the slow-client refresh policy).
func dialLive(t *testing.T, base string, cookie *http.Cookie, slow bool) *liveClientConn {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/api/_live", nil)
	if err != nil {
		cancel()
		t.Fatalf("request: %v", err)
	}
	req.AddCookie(cookie)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		cancel()
		t.Fatalf("dial live: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Fatalf("live status = %d, want 200", resp.StatusCode)
	}
	c := &liveClientConn{resp: resp, frames: make(chan sseFrame, 256), cancel: cancel}
	go c.read(slow)
	return c
}

func (c *liveClientConn) read(slow bool) {
	defer close(c.frames)
	defer c.resp.Body.Close()
	reader := bufio.NewReader(c.resp.Body)
	var event string
	var data []string
	seen := 0
	for {
		line, err := reader.ReadString('\n')
		if err != nil {
			return
		}
		line = strings.TrimRight(line, "\r\n")
		if line == "" {
			if len(data) > 0 {
				c.frames <- sseFrame{event: event, data: strings.Join(data, "\n")}
				seen++
				if slow && seen == 1 {
					time.Sleep(2 * time.Second) // stall so the server buffer overflows
				}
			}
			event, data = "", nil
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
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

func (c *liveClientConn) close() { c.cancel() }

// nextEvent waits for the next frame with a non-empty event name (skipping the
// heartbeat comments the reader already drops — comments never produce
// frames).
func (c *liveClientConn) nextEvent(t *testing.T, timeout time.Duration) sseFrame {
	t.Helper()
	deadline := time.After(timeout)
	for {
		select {
		case f, ok := <-c.frames:
			if !ok {
				t.Fatal("live stream closed before expected event")
			}
			return f
		case <-deadline:
			t.Fatal("timed out waiting for live event")
		}
	}
}

func waitFor(t *testing.T, timeout time.Duration, cond func() bool) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatal("condition not met before timeout")
}

func TestLiveRefreshOnSubscribeAndVerbatimRelay(t *testing.T) {
	up := newFakeUpstream(t)
	app, server, key := newLiveTestApp(t, up.server.URL)
	cookie := liveSessionCookie(t, app, key)

	c := dialLive(t, server.URL, cookie, false)
	defer c.close()

	// Every fresh subscriber gets a synthetic refresh first.
	if f := c.nextEvent(t, 2*time.Second); f.event != "refresh" || f.data != "{}" {
		t.Fatalf("first event = %+v, want refresh {}", f)
	}

	// Upstream events relay verbatim (event name + data byte-for-byte).
	want := sseFrame{event: "rows_changed", data: `{"type":"rows_changed","page_id":"p1","action":"update"}`}
	up.push(want)
	if f := c.nextEvent(t, 2*time.Second); f != want {
		t.Fatalf("relayed = %+v, want %+v", f, want)
	}

	// The upstream carried the app secret as its bearer.
	waitFor(t, time.Second, func() bool {
		up.mu.Lock()
		defer up.mu.Unlock()
		return up.lastBearer == "Bearer hvapp_livesecret"
	})
}

func TestLiveSingleUpstreamForManyClients(t *testing.T) {
	up := newFakeUpstream(t)
	app, server, key := newLiveTestApp(t, up.server.URL)
	cookie := liveSessionCookie(t, app, key)

	var conns []*liveClientConn
	for i := 0; i < 4; i++ {
		c := dialLive(t, server.URL, cookie, false)
		defer c.close()
		if f := c.nextEvent(t, 2*time.Second); f.event != "refresh" {
			t.Fatalf("client %d first event = %+v, want refresh", i, f)
		}
		conns = append(conns, c)
	}

	// One upstream event reaches all four browsers.
	up.push(sseFrame{event: "pages_changed", data: `{"type":"pages_changed"}`})
	for i, c := range conns {
		if f := c.nextEvent(t, 2*time.Second); f.event != "pages_changed" {
			t.Fatalf("client %d relayed = %+v", i, f)
		}
	}

	// Regardless of client count, exactly one upstream connection is held.
	if peak := up.peakConns(); peak != 1 {
		t.Fatalf("peak upstream connections = %d, want 1", peak)
	}
}

func TestLiveRefreshAfterReconnect(t *testing.T) {
	prev := liveUpstreamInitialBackoff
	liveUpstreamInitialBackoff = 10 * time.Millisecond
	t.Cleanup(func() { liveUpstreamInitialBackoff = prev })
	up := newFakeUpstream(t)
	app, server, key := newLiveTestApp(t, up.server.URL)
	cookie := liveSessionCookie(t, app, key)

	c := dialLive(t, server.URL, cookie, false)
	defer c.close()
	if f := c.nextEvent(t, 2*time.Second); f.event != "refresh" {
		t.Fatalf("subscribe refresh missing: %+v", f)
	}
	// Ensure the upstream is established before dropping it.
	waitFor(t, 2*time.Second, func() bool { a, _ := up.snapshot(); return a == 1 })

	up.dropConnection()

	// After the relay reconnects, every client receives a synthetic refresh.
	if f := c.nextEvent(t, 3*time.Second); f.event != "refresh" {
		t.Fatalf("post-reconnect event = %+v, want refresh", f)
	}
	// A second upstream connection was made (reconnect).
	waitFor(t, 2*time.Second, func() bool { _, total := up.snapshot(); return total >= 2 })
}

func TestLiveSlowClientForcedRefresh(t *testing.T) {
	up := newFakeUpstream(t)
	app, server, key := newLiveTestApp(t, up.server.URL)
	cookie := liveSessionCookie(t, app, key)

	// This client stalls after its first frame, so the server-side buffer
	// (liveClientBuffer) overflows as events pile up.
	c := dialLive(t, server.URL, cookie, true)
	defer c.close()
	if f := c.nextEvent(t, 2*time.Second); f.event != "refresh" {
		t.Fatalf("subscribe refresh missing: %+v", f)
	}

	// Flood the stalled client with far more data than its buffer plus the OS
	// socket buffers can hold. Large payloads guarantee real backpressure, so
	// the relay drops events and arms a forced refresh instead of blocking the
	// fan-out.
	big := strings.Repeat("x", 32*1024)
	for i := 0; i < 500; i++ {
		up.push(sseFrame{event: "rows_changed", data: big})
	}

	// Once it resumes, a forced refresh appears among the relayed events: the
	// relay dropped what it couldn't buffer and told the client to resync.
	sawRefresh := false
	deadline := time.After(6 * time.Second)
	for !sawRefresh {
		select {
		case f, ok := <-c.frames:
			if !ok {
				t.Fatal("stream closed before forced refresh")
			}
			if f.event == "refresh" {
				sawRefresh = true
			}
		case <-deadline:
			t.Fatal("no forced refresh for slow client")
		}
	}
}

func TestLiveUpstreamTeardownAfterLastClient(t *testing.T) {
	prev := liveUpstreamLinger
	liveUpstreamLinger = 100 * time.Millisecond
	t.Cleanup(func() { liveUpstreamLinger = prev })
	up := newFakeUpstream(t)
	app, server, key := newLiveTestApp(t, up.server.URL)
	cookie := liveSessionCookie(t, app, key)

	c := dialLive(t, server.URL, cookie, false)
	if f := c.nextEvent(t, 2*time.Second); f.event != "refresh" {
		t.Fatalf("subscribe refresh missing: %+v", f)
	}
	waitFor(t, 2*time.Second, func() bool { a, _ := up.snapshot(); return a == 1 })

	// Last client leaves; upstream lingers briefly, then tears down.
	c.close()
	waitFor(t, 2*time.Second, func() bool { a, _ := up.snapshot(); return a == 0 })
}
