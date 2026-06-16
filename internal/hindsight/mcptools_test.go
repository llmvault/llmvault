package hindsight

import (
	"strings"
	"testing"
)

func TestMemoryRetainResponseExplainsBackgroundProcessing(t *testing.T) {
	resp := memoryRetainResponse("org-test", "manual-agent-doc", &RetainResponse{
		Success:     true,
		BankID:      "org-test",
		ItemsCount:  1,
		Async:       true,
		OperationID: "retain-op-1",
	})
	if !resp.Async {
		t.Fatal("expected async retain response")
	}
	if !strings.Contains(resp.Message, "processed in the background") {
		t.Fatalf("message does not explain background processing: %q", resp.Message)
	}
	if !strings.Contains(resp.Message, "memory_recall") {
		t.Fatalf("message does not explain delayed recall visibility: %q", resp.Message)
	}
}
