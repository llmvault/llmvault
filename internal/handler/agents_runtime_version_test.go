package handler

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

func TestAgentRuntimeVersionLabel(t *testing.T) {
	tests := []struct {
		name string
		ref  string
		want string
	}{
		{
			name: "microsandbox snapshot alias",
			ref:  "hivy-sandboxes-runtime-v3-2-1-amd64-small",
			want: "v3.2.1",
		},
		{
			name: "image tag",
			ref:  "ghcr.io/usehivy/hivy-sandboxes-runtime:v3.2.1-amd64",
			want: "v3.2.1",
		},
		{
			name: "unversioned local runtime",
			ref:  "hivy-sandboxes-runtime:runtime",
			want: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := agentRuntimeVersionLabel(tt.ref); got != tt.want {
				t.Fatalf("agentRuntimeVersionLabel(%q) = %q, want %q", tt.ref, got, tt.want)
			}
		})
	}
}

func TestSandboxSummaryIncludesRuntimeVersion(t *testing.T) {
	snapshotID := "hivy-sandboxes-runtime-v3-2-1-amd64-small"
	sb := model.Sandbox{
		ID:         uuid.New(),
		Status:     "running",
		ExternalID: "external-sandbox",
		SnapshotID: &snapshotID,
		CreatedAt:  time.Date(2026, 6, 17, 10, 0, 0, 0, time.UTC),
	}

	summary := sandboxSummary(sb)
	if summary.RuntimeVersion != "v3.2.1" {
		t.Fatalf("RuntimeVersion = %q, want v3.2.1", summary.RuntimeVersion)
	}
}
