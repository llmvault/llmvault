package e2e

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

type agentSessionsSandboxAccess struct {
	SandboxBaseURL string   `json:"sandbox_base_url"`
	Token          string   `json:"token"`
	Scopes         []string `json:"scopes"`
}

func fetchAgentSessionsSandboxAccess(t *testing.T, ctx context.Context, baseURL, token, orgID, sessionID string) agentSessionsSandboxAccess {
	t.Helper()
	endpoint := baseURL + "/v1/sessions/" + sessionID + "/sandbox-access"
	deadline := time.Now().Add(3 * time.Minute)
	var lastStatus int
	var lastBody []byte
	for {
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
		raw, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var out agentSessionsSandboxAccess
			if err := json.Unmarshal(raw, &out); err != nil {
				t.Fatalf("decode sandbox access: %v\n%s", err, raw)
			}
			if out.Token == "" {
				t.Fatalf("sandbox access missing token: %+v", out)
			}
			if sandboxRuntimeHealthy(ctx, out.SandboxBaseURL) {
				return out
			}
			lastStatus = http.StatusServiceUnavailable
			lastBody = []byte("sandbox runtime healthz is not ready")
		} else {
			lastStatus = resp.StatusCode
			lastBody = raw
		}
		if resp.StatusCode != http.StatusNotFound || time.Now().After(deadline) {
			if lastStatus != http.StatusServiceUnavailable || time.Now().After(deadline) {
				t.Fatalf("sandbox access status=%d body=%s", lastStatus, lastBody)
			}
		}
		select {
		case <-ctx.Done():
			t.Fatalf("sandbox access wait canceled after status=%d body=%s: %v", lastStatus, lastBody, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func sandboxRuntimeHealthy(ctx context.Context, sandboxBaseURL string) bool {
	reqCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(reqCtx, http.MethodGet, strings.TrimRight(sandboxBaseURL, "/")+"/healthz", nil)
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
