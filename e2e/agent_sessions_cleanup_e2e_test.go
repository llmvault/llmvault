package e2e

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func agentSessionsDeleteSandbox(t *testing.T, ctx context.Context, baseURL, token, orgID, sandboxID string) {
	t.Helper()
	if sandboxID == "" {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, baseURL+"/v1/sandboxes/"+sandboxID, nil)
	if err != nil {
		t.Logf("cleanup sandbox %s: build request: %v", sandboxID, err)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Org-ID", orgID)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Logf("cleanup sandbox %s: %v", sandboxID, err)
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
		body, _ := io.ReadAll(resp.Body)
		t.Logf("cleanup sandbox %s: status=%d body=%s", sandboxID, resp.StatusCode, body)
	}
}
