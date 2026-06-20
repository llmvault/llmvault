package e2e

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"
	"time"
)

func doRuntimeJSON(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, method, url, token string, body []byte, wantStatus int) []byte {
	t.Helper()
	trace.Body("runtime-http", method+" "+url+" request", body)
	req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
	if err != nil {
		t.Fatalf("new runtime request: %v", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("%s %s failed: %v", method, url, err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	trace.Body("runtime-http", fmt.Sprintf("%s %s response status=%d", method, url, resp.StatusCode), respBody)
	if resp.StatusCode != wantStatus {
		t.Fatalf("%s %s status=%d want=%d body=%s", method, url, resp.StatusCode, wantStatus, respBody)
	}
	return respBody
}

func waitForRuntimeReady(t *testing.T, trace *agentRuntimeE2ETrace, ctx context.Context, baseURL, token string) {
	t.Helper()
	deadline := time.Now().Add(60 * time.Second)
	lastLog := time.Time{}
	for time.Now().Before(deadline) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/readyz", nil)
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				trace.Logf("runtime", "readyz ok base_url=%s", baseURL)
				return
			}
		}
		if time.Since(lastLog) > 2*time.Second {
			trace.Logf("runtime", "waiting for readyz base_url=%s err=%v", baseURL, err)
			lastLog = time.Now()
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("runtime did not become ready")
}
