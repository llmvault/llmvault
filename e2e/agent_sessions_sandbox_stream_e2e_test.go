package e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"
)

func agentSessionsReadSandboxStream(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string) []runtimeSSEEvent {
	t.Helper()
	stream := agentSessionsOpenSandboxStream(t, ctx, apiBase, token, orgID, sessionID, true)
	return stream.waitDone(t, ctx)
}

type agentSessionsLiveSandboxStream struct {
	events chan runtimeSSEEvent
	done   chan []runtimeSSEEvent
	errs   chan error
}

func agentSessionsStartSandboxStream(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string) *agentSessionsLiveSandboxStream {
	t.Helper()
	return agentSessionsOpenSandboxStream(t, ctx, apiBase, token, orgID, sessionID, false)
}

func agentSessionsStartSandboxStreamWithAccess(t *testing.T, ctx context.Context, sessionID string, access agentSessionsSandboxAccess) *agentSessionsLiveSandboxStream {
	t.Helper()
	return agentSessionsOpenSandboxStreamWithAccess(t, ctx, sessionID, access, false)
}

func agentSessionsStartSandboxStreamFromTurnFollowWithAccess(t *testing.T, ctx context.Context, sessionID string, access agentSessionsSandboxAccess, turnID string) *agentSessionsLiveSandboxStream {
	t.Helper()
	if turnID == "" {
		t.Fatal("direct sandbox stream from turn follow requires turn id")
	}
	return agentSessionsOpenSandboxStreamWithAccessAndQuery(t, ctx, sessionID, access, false, url.Values{
		"follow":       []string{"true"},
		"from_turn_id": []string{turnID},
	})
}

func agentSessionsOpenSandboxStream(t *testing.T, ctx context.Context, apiBase, token, orgID, sessionID string, stopOnTerminal bool) *agentSessionsLiveSandboxStream {
	t.Helper()
	access := fetchAgentSessionsSandboxAccess(t, ctx, apiBase, token, orgID, sessionID)
	return agentSessionsOpenSandboxStreamWithAccess(t, ctx, sessionID, access, stopOnTerminal)
}

func agentSessionsOpenSandboxStreamWithAccess(t *testing.T, ctx context.Context, sessionID string, access agentSessionsSandboxAccess, stopOnTerminal bool) *agentSessionsLiveSandboxStream {
	t.Helper()
	return agentSessionsOpenSandboxStreamWithAccessAndQuery(t, ctx, sessionID, access, stopOnTerminal, url.Values{"after_seq": []string{"0"}})
}

func agentSessionsOpenSandboxStreamWithAccessAndQuery(t *testing.T, ctx context.Context, sessionID string, access agentSessionsSandboxAccess, stopOnTerminal bool, values url.Values) *agentSessionsLiveSandboxStream {
	t.Helper()
	requireAgentSessionsSandboxStreamAccess(t, access, sessionID)
	resp := openSandboxSessionStreamWithRetry(t, ctx, sessionID, access, values) //nolint:bodyclose // Closed by the stream reader goroutine.

	stream := &agentSessionsLiveSandboxStream{
		events: make(chan runtimeSSEEvent, 256),
		done:   make(chan []runtimeSSEEvent, 1),
		errs:   make(chan error, 1),
	}
	go func() {
		defer resp.Body.Close()
		events, err := readAgentSessionsSandboxStreamEvents(resp.Body, stream.events, stopOnTerminal)
		if err != nil {
			stream.errs <- err
			return
		}
		stream.done <- events
	}()
	return stream
}

func openSandboxSessionStreamWithRetry(t *testing.T, ctx context.Context, sessionID string, access agentSessionsSandboxAccess, values url.Values) *http.Response {
	t.Helper()
	resp, err := openSandboxSessionStreamClientWithQuery(ctx, sessionID, access, values)
	if err != nil {
		t.Fatal(err)
	}
	return resp
}

func openSandboxSessionStreamClient(ctx context.Context, sessionID string, access agentSessionsSandboxAccess) (*http.Response, error) {
	return openSandboxSessionStreamClientWithQuery(ctx, sessionID, access, url.Values{"after_seq": []string{"0"}})
}

func openSandboxSessionStreamClientWithQuery(ctx context.Context, sessionID string, access agentSessionsSandboxAccess, values url.Values) (*http.Response, error) {
	endpoint, err := agentSessionsSandboxStreamURL(access, sessionID, values)
	if err != nil {
		return nil, err
	}
	deadline := time.Now().Add(2 * time.Minute)
	var lastStatus int
	var lastBody []byte
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
		if err != nil {
			return nil, fmt.Errorf("new direct sandbox stream request: %w", err)
		}
		req.Header.Set("Accept", "text/event-stream")
		req.Header.Set("Authorization", "Bearer "+access.Token)
		resp, err := http.DefaultClient.Do(req)
		if err == nil && resp.StatusCode == http.StatusOK {
			return resp, nil
		}
		if err != nil {
			lastErr = err
		} else {
			lastStatus = resp.StatusCode
			lastBody, _ = io.ReadAll(resp.Body)
			resp.Body.Close()
		}
		if time.Now().After(deadline) {
			if lastErr != nil {
				return nil, fmt.Errorf("open direct sandbox stream: %w", lastErr)
			}
			return nil, fmt.Errorf("direct sandbox stream status=%d body=%s", lastStatus, lastBody)
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return nil, fmt.Errorf("direct sandbox stream wait canceled: %w", lastErr)
			}
			return nil, fmt.Errorf("direct sandbox stream wait canceled after status=%d body=%s: %w", lastStatus, lastBody, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func (s *agentSessionsLiveSandboxStream) waitForEvent(t *testing.T, ctx context.Context, timeout time.Duration, want func(runtimeSSEEvent) bool) runtimeSSEEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	observed := make([]runtimeSSEEvent, 0, 32)
	for {
		select {
		case event := <-s.events:
			observed = append(observed, event)
			if want(event) {
				return event
			}
		case events := <-s.done:
			t.Fatalf("direct sandbox stream completed before expected event; events=%s", summarizeRuntimeSSEEvents(events))
		case err := <-s.errs:
			t.Fatalf("direct sandbox stream failed before expected event: %v", err)
		case <-ctx.Done():
			t.Fatalf("direct sandbox stream context ended before expected event: %v", ctx.Err())
		case <-timer.C:
			t.Fatalf("timed out waiting for direct sandbox stream event; observed=%s", summarizeRuntimeSSEEvents(observed))
		}
	}
}

func (s *agentSessionsLiveSandboxStream) waitDone(t *testing.T, ctx context.Context) []runtimeSSEEvent {
	t.Helper()
	for {
		select {
		case events := <-s.done:
			return events
		case err := <-s.errs:
			t.Fatalf("read direct sandbox stream: %v", err)
		case <-ctx.Done():
			t.Fatalf("direct sandbox stream context ended before done: %v", ctx.Err())
		}
	}
}

func (s *agentSessionsLiveSandboxStream) assertNoBufferedEvent(t *testing.T, want func(runtimeSSEEvent) bool) {
	t.Helper()
	for {
		select {
		case event := <-s.events:
			if want(event) {
				t.Fatalf("direct sandbox stream had unexpected buffered event: %s", event.RawData)
			}
		default:
			return
		}
	}
}
