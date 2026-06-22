package e2e

import (
	"context"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

type directRuntimeSSEResult struct {
	events []runtimeSSEEvent
	err    error
}

type directRuntimeSSEHandle struct {
	ch <-chan directRuntimeSSEResult
}

type directRuntimeLiveStream struct {
	events chan runtimeSSEEvent
	done   chan []runtimeSSEEvent
	errs   chan error
}

func readDirectRuntimeSSEAsync(trace *agentRuntimeE2ETrace, ctx context.Context, streamURL, token string) directRuntimeSSEHandle {
	ch := make(chan directRuntimeSSEResult, 1)
	go func() {
		events, err := readRuntimeSSEClient(ctx, trace, "direct-response", streamURL, token, nil)
		ch <- directRuntimeSSEResult{events: events, err: err}
	}()
	return directRuntimeSSEHandle{ch: ch}
}

func (h directRuntimeSSEHandle) wait(t *testing.T) []runtimeSSEEvent {
	t.Helper()
	result := <-h.ch
	if result.err != nil {
		t.Fatalf("direct runtime stream failed: %v", result.err)
	}
	return result.events
}

func startDirectRuntimeLiveStream(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, streamURL, token string) *directRuntimeLiveStream {
	t.Helper()
	stream := &directRuntimeLiveStream{
		events: make(chan runtimeSSEEvent, 256),
		done:   make(chan []runtimeSSEEvent, 1),
		errs:   make(chan error, 1),
	}
	go func() {
		events, err := readRuntimeSSEClient(ctx, trace, "direct-runtime", streamURL, token, func(event runtimeSSEEvent) {
			select {
			case stream.events <- event:
			default:
			}
		})
		if err != nil {
			stream.errs <- err
			return
		}
		stream.done <- events
	}()
	return stream
}

func (s *directRuntimeLiveStream) waitForEvent(t *testing.T, ctx context.Context, timeout time.Duration, want func(runtimeSSEEvent) bool) runtimeSSEEvent {
	t.Helper()
	timer := time.NewTimer(timeout)
	defer timer.Stop()
	for {
		select {
		case event := <-s.events:
			if want(event) {
				return event
			}
		case events := <-s.done:
			t.Fatalf("direct runtime stream completed before expected event; events=%s", summarizeRuntimeSSEEvents(events))
		case err := <-s.errs:
			t.Fatalf("direct runtime stream failed before expected event: %v", err)
		case <-ctx.Done():
			t.Fatalf("direct runtime stream context ended before expected event: %v", ctx.Err())
		case <-timer.C:
			t.Fatalf("timed out waiting for direct runtime stream event")
		}
	}
}

func directRuntimeStreamURL(t *testing.T, baseURL, streamPath string) string {
	t.Helper()
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + "/" + strings.TrimLeft(streamPath, "/"))
	if err != nil {
		t.Fatalf("parse direct stream url: %v", err)
	}
	return parsed.String()
}

func directRuntimeJWT(t *testing.T, runtimeSecret, sessionID, sandboxID string, scopes ...string) string {
	t.Helper()
	if len(scopes) == 0 {
		scopes = []string{"stream:read", "repo:read"}
	}
	now := time.Now().UTC()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"iss":        "hivy",
		"aud":        "hivy-runtime",
		"sub":        "e2e-user",
		"org_id":     "e2e-org",
		"session_id": sessionID,
		"sandbox_id": sandboxID,
		"scopes":     scopes,
		"iat":        now.Unix(),
		"nbf":        now.Add(-time.Second).Unix(),
		"exp":        now.Add(time.Hour).Unix(),
		"jti":        "e2e-" + sessionID,
	})
	signed, err := token.SignedString([]byte(runtimeSecret))
	if err != nil {
		t.Fatalf("sign direct runtime jwt: %v", err)
	}
	return signed
}

func assertDirectStreamDisabledBeforeConfig(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL string) {
	t.Helper()
	streamURL := directRuntimeStreamURL(t, baseURL, "/sessions/not-yet-configured/stream")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, streamURL, nil)
	if err != nil {
		t.Fatalf("new pre-config direct stream request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("pre-config direct stream request failed: %v", err)
	}
	defer resp.Body.Close()
	trace.Logf("direct-stream", "pre-config browser stream status=%d", resp.StatusCode)
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("pre-config direct stream status=%d want=%d", resp.StatusCode, http.StatusUnauthorized)
	}
}
