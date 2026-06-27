package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

type agentSessionsSandboxAccess struct {
	SessionID      string   `json:"session_id"`
	SandboxID      string   `json:"sandbox_id"`
	SandboxBaseURL string   `json:"sandbox_base_url"`
	Token          string   `json:"token"`
	ExpiresAt      string   `json:"expires_at"`
	Scopes         []string `json:"scopes"`
}

func fetchAgentSessionsSandboxAccess(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID string) agentSessionsSandboxAccess {
	t.Helper()
	out, err := fetchAgentSessionsSandboxAccessClient(ctx, baseURL, token, orgID, sessionID)
	if err != nil {
		t.Fatalf("sandbox access: %v", err)
	}
	return out
}

func fetchAgentSessionsSandboxAccessClient(ctx context.Context, baseURL, token, orgID, sessionID string) (agentSessionsSandboxAccess, error) {
	endpoint := strings.TrimRight(baseURL, "/") + "/v1/sessions/" + url.PathEscape(sessionID) + "/sandbox-access"
	deadline := time.Now().Add(3 * time.Minute)
	var lastStatus int
	var lastBody []byte
	var lastErr error
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
		if err != nil {
			return agentSessionsSandboxAccess{}, fmt.Errorf("new sandbox access request: %w", err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("X-Org-ID", orgID)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			lastErr = err
		} else {
			raw, _ := io.ReadAll(resp.Body)
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				var out agentSessionsSandboxAccess
				if err := json.Unmarshal(raw, &out); err != nil {
					return agentSessionsSandboxAccess{}, fmt.Errorf("decode sandbox access: %w\n%s", err, raw)
				}
				if out.Token == "" {
					return agentSessionsSandboxAccess{}, fmt.Errorf("sandbox access missing token: %+v", out)
				}
				if sandboxRuntimeHealthy(ctx, out.SandboxBaseURL) {
					return out, nil
				}
				lastStatus = http.StatusServiceUnavailable
				lastBody = []byte("sandbox runtime healthz is not ready")
			} else {
				lastStatus = resp.StatusCode
				lastBody = raw
			}
		}
		if lastStatus != http.StatusNotFound || time.Now().After(deadline) {
			if lastStatus != http.StatusServiceUnavailable || time.Now().After(deadline) {
				if lastErr != nil {
					return agentSessionsSandboxAccess{}, fmt.Errorf("sandbox access request failed: %w", lastErr)
				}
				return agentSessionsSandboxAccess{}, fmt.Errorf("sandbox access status=%d body=%s", lastStatus, lastBody)
			}
		}
		select {
		case <-ctx.Done():
			if lastErr != nil {
				return agentSessionsSandboxAccess{}, fmt.Errorf("sandbox access wait canceled after error: %w: %w", lastErr, ctx.Err())
			}
			return agentSessionsSandboxAccess{}, fmt.Errorf("sandbox access wait canceled after status=%d body=%s: %w", lastStatus, lastBody, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func requireAgentSessionsSandboxStreamAccess(t *testing.T, access agentSessionsSandboxAccess, sessionID string) {
	t.Helper()
	if err := validateAgentSessionsSandboxStreamAccess(access, sessionID); err != nil {
		t.Fatal(err)
	}
}

func validateAgentSessionsSandboxStreamAccess(access agentSessionsSandboxAccess, sessionID string) error {
	if access.SessionID != "" && access.SessionID != sessionID {
		return fmt.Errorf("sandbox access session_id=%s want %s", access.SessionID, sessionID)
	}
	if access.SandboxID == "" {
		return fmt.Errorf("sandbox access missing sandbox_id: %+v", access)
	}
	if strings.TrimSpace(access.SandboxBaseURL) == "" {
		return fmt.Errorf("sandbox access missing sandbox_base_url: %+v", access)
	}
	if strings.TrimSpace(access.Token) == "" {
		return fmt.Errorf("sandbox access missing token: %+v", access)
	}
	if !agentSessionsAccessHasScope(access.Scopes, "stream:read") {
		return fmt.Errorf("sandbox access scopes=%v missing stream:read for direct runtime session stream", access.Scopes)
	}
	return nil
}

func agentSessionsAccessHasScope(scopes []string, want string) bool {
	for _, scope := range scopes {
		if strings.TrimSpace(scope) == want {
			return true
		}
	}
	return false
}

func agentSessionsSandboxStreamURL(access agentSessionsSandboxAccess, sessionID string, values url.Values) (string, error) {
	baseURL := agentSessionsHostReachableURL(access.SandboxBaseURL)
	parsed, err := url.Parse(strings.TrimRight(baseURL, "/") + "/sessions/" + url.PathEscape(sessionID) + "/stream")
	if err != nil {
		return "", fmt.Errorf("parse sandbox stream url: %w", err)
	}
	parsed.RawQuery = values.Encode()
	return parsed.String(), nil
}

func sandboxRuntimeHealthy(ctx context.Context, sandboxBaseURL string) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	baseURL := agentSessionsHostReachableURL(sandboxBaseURL)
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimRight(baseURL, "/")+"/healthz", nil)
	if err != nil {
		return false
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func agentSessionsSandboxAccessStatus(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID string, want int) {
	t.Helper()
	endpoint := baseURL + "/v1/sessions/" + sessionID + "/sandbox-access"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, nil)
	if err != nil {
		t.Fatalf("new sandbox access request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-ID", orgID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("sandbox access request failed: %v", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("sandbox access status=%d want=%d body=%s", resp.StatusCode, want, raw)
	}
}
