package e2e

import (
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func (cp *agentRuntimeMockControlPlane) waitForActivity(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	lastLog := time.Time{}
	for time.Now().Before(deadline) {
		cp.mu.Lock()
		presign := cp.presignRequests
		upload := cp.uploadRequests
		confirm := cp.confirmRequests
		webhook := cp.webhookRequests
		batch := cp.batchRequests
		cp.mu.Unlock()
		if webhook > 0 {
			cp.trace.Logf("assert", "control-plane activity complete presign=%d upload=%d confirm=%d webhook=%d batch=%d", presign, upload, confirm, webhook, batch)
			return
		}
		if time.Since(lastLog) > 2*time.Second {
			cp.trace.Logf("control-plane", "waiting activity presign=%d upload=%d confirm=%d webhook=%d batch=%d", presign, upload, confirm, webhook, batch)
			lastLog = time.Now()
		}
		time.Sleep(250 * time.Millisecond)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	t.Fatalf("mock control plane webhook activity incomplete: presign=%d upload=%d confirm=%d webhook=%d batch=%d paths=%v", cp.presignRequests, cp.uploadRequests, cp.confirmRequests, cp.webhookRequests, cp.batchRequests, cp.paths)
}

func (cp *agentRuntimeMockControlPlane) assertAgentActivity(t *testing.T) {
	t.Helper()
	cp.mu.Lock()
	paths := append([]string(nil), cp.paths...)
	cp.mu.Unlock()
	if !strings.Contains(strings.Join(paths, "\n"), "/internal/webhooks/agent/"+cp.sandboxID) {
		t.Fatalf("control-plane did not receive agent webhook path: %v", paths)
	}
	cp.trace.Logf("assert", "control-plane agent webhook path received paths=%v", paths)
}

func (cp *agentRuntimeMockControlPlane) waitForBatchActivity(t *testing.T) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	lastLog := time.Time{}
	for time.Now().Before(deadline) {
		cp.mu.Lock()
		batch := cp.batchRequests
		eventTypes := append([]string(nil), cp.eventTypes...)
		cp.mu.Unlock()
		if batch > 0 {
			cp.trace.Logf("assert", "control-plane batch webhook received batches=%d", batch)
			return
		}
		if time.Since(lastLog) > 2*time.Second {
			cp.trace.Logf("control-plane", "waiting for batch webhook event_types=%v", eventTypes)
			lastLog = time.Now()
		}
		time.Sleep(250 * time.Millisecond)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	t.Fatalf("control-plane did not receive batch webhook; paths=%v event_types=%v", cp.paths, cp.eventTypes)
}

func (cp *agentRuntimeMockControlPlane) waitForWebhookEventType(t *testing.T, eventType string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	lastLog := time.Time{}
	for time.Now().Before(deadline) {
		cp.mu.Lock()
		seen := containsString(cp.eventTypes, eventType)
		eventTypes := append([]string(nil), cp.eventTypes...)
		cp.mu.Unlock()
		if seen {
			cp.trace.Logf("assert", "control-plane webhook event observed event_type=%s", eventType)
			return
		}
		if time.Since(lastLog) > 2*time.Second {
			cp.trace.Logf("control-plane", "waiting for webhook event_type=%s seen=%v", eventType, eventTypes)
			lastLog = time.Now()
		}
		time.Sleep(250 * time.Millisecond)
	}
	cp.mu.Lock()
	defer cp.mu.Unlock()
	t.Fatalf("control-plane did not receive webhook event_type=%s; event_types=%v payloads=%v", eventType, cp.eventTypes, cp.payloads)
}

func (cp *agentRuntimeMockControlPlane) webhookEventsByType(eventType string) []map[string]any {
	cp.mu.Lock()
	payloads := append([]string(nil), cp.payloads...)
	cp.mu.Unlock()
	var events []map[string]any
	for _, payload := range payloads {
		var event map[string]any
		if err := json.Unmarshal([]byte(payload), &event); err != nil {
			continue
		}
		if got, _ := event["event_type"].(string); got == eventType {
			events = append(events, event)
		}
	}
	return events
}

func (cp *agentRuntimeMockControlPlane) waitForWebhookPayloadContaining(t *testing.T, eventType, marker string) {
	t.Helper()
	deadline := time.Now().Add(90 * time.Second)
	lastLog := time.Time{}
	for time.Now().Before(deadline) {
		events := cp.webhookEventsByType(eventType)
		for _, event := range events {
			body, _ := json.Marshal(event)
			if strings.Contains(string(body), marker) {
				cp.trace.Logf("assert", "control-plane webhook payload observed event_type=%s marker=%s", eventType, marker)
				return
			}
		}
		if time.Since(lastLog) > 2*time.Second {
			cp.trace.Logf("control-plane", "waiting for webhook payload event_type=%s marker=%s events=%d", eventType, marker, len(events))
			lastLog = time.Now()
		}
		time.Sleep(250 * time.Millisecond)
	}
	t.Fatalf("control-plane did not receive webhook payload event_type=%s marker=%s events=%v", eventType, marker, cp.webhookEventsByType(eventType))
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
