package hivycore

import (
	"strings"
	"testing"
	"time"
)

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
