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
		t.Fatalf("direct sandbox stream failed: %v", result.err)
	}
	return result.events
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
