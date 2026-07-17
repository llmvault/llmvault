package daytona

import (
	"testing"

	"github.com/usehivy/hivy/internal/sandbox"
)

func TestResolveRuntimeSnapshotRef(t *testing.T) {
	tests := []struct {
		name      string
		image     string
		cpu       int
		memory    int
		disk      int
		want      string
		wantError bool
	}{
		{
			name:   "default nano release",
			image:  defaultRuntimeRepository + ":v7.2.1-amd64",
			cpu:    1,
			memory: 1,
			disk:   5,
			want:   defaultSnapshotPrefix + "-7-2-1-nano-v1",
		},
		{
			name:   "developer release",
			image:  developerRuntimeRepository + ":v7.2.1",
			cpu:    2,
			memory: 4,
			disk:   20,
			want:   developerSnapshotPrefix + "-7-2-1-medium-v1",
		},
		{
			name:   "custom snapshot passes through",
			image:  "org-custom-snapshot",
			cpu:    1,
			memory: 2,
			disk:   10,
			want:   "org-custom-snapshot",
		},
		{
			name:   "micro uses its Daytona-specific snapshot",
			image:  defaultRuntimeRepository + ":v7.2.1",
			cpu:    1,
			memory: 0,
			disk:   5,
			want:   defaultSnapshotPrefix + "-7-2-1-micro-v1",
		},
		{
			name:   "xlarge maps to its snapshot before Daytona disk adjustment",
			image:  defaultRuntimeRepository + ":v7.2.1",
			cpu:    8,
			memory: 16,
			disk:   60,
			want:   defaultSnapshotPrefix + "-7-2-1-xlarge-v1",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := resolveRuntimeSnapshotRef(sandbox.CreateSandboxOpts{
				TemplateRef: tt.image,
				CPU:         tt.cpu,
				Memory:      tt.memory,
				Disk:        tt.disk,
			})
			if tt.wantError {
				if err == nil {
					t.Fatalf("expected error, got snapshot %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolve snapshot: %v", err)
			}
			if got != tt.want {
				t.Fatalf("snapshot = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestDaytonaRuntimeResourceAdjustment(t *testing.T) {
	tests := []struct {
		name       string
		image      string
		cpu        int
		memory     int
		disk       int
		wantMemory int
		wantDisk   int
	}{
		{
			name:       "default micro gets Daytona minimum memory",
			image:      defaultRuntimeRepository + ":v7.2.1-amd64",
			cpu:        1,
			memory:     0,
			disk:       5,
			wantMemory: 1,
			wantDisk:   5,
		},
		{
			name:       "developer nano gets image minimum disk",
			image:      developerRuntimeRepository + ":v7.2.1-amd64",
			cpu:        1,
			memory:     1,
			disk:       5,
			wantMemory: 1,
			wantDisk:   10,
		},
		{
			name:       "medium disk is capped at account maximum",
			image:      defaultRuntimeRepository + ":v7.2.1-amd64",
			cpu:        2,
			memory:     4,
			disk:       20,
			wantMemory: 4,
			wantDisk:   10,
		},
		{
			name:       "xlarge keeps cpu and memory while disk is capped",
			image:      defaultRuntimeRepository + ":v7.2.1-amd64",
			cpu:        8,
			memory:     16,
			disk:       60,
			wantMemory: 16,
			wantDisk:   10,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := daytonaRuntimeResourceAdjustment(sandbox.CreateSandboxOpts{
				TemplateRef: tt.image,
				CPU:         tt.cpu,
				Memory:      tt.memory,
				Disk:        tt.disk,
			})
			if !ok {
				t.Fatal("expected Hivy runtime adjustment")
			}
			if got.Memory != tt.wantMemory || got.Disk != tt.wantDisk {
				t.Fatalf("adjusted resources = memory %d disk %d, want memory %d disk %d",
					got.Memory, got.Disk, tt.wantMemory, tt.wantDisk)
			}
		})
	}
}

func TestIsHivyRuntimeImageRef(t *testing.T) {
	for _, image := range []string{
		defaultRuntimeRepository + ":v7.2.1-amd64",
		developerRuntimeRepository + ":v7.2.1",
		defaultDaytonaRuntimeRepository + ":v7.2.1-amd64",
	} {
		if !isHivyRuntimeImageRef(image) {
			t.Fatalf("expected %q to be a Hivy runtime image", image)
		}
	}
	if isHivyRuntimeImageRef("node:22-bookworm-slim") {
		t.Fatal("did not expect node image to be a Hivy runtime image")
	}
}

func TestNonEmptyCommands(t *testing.T) {
	got := nonEmptyCommands([]string{" npm install ", "", "  ", "npm test"})
	want := []string{"npm install", "npm test"}
	if len(got) != len(want) {
		t.Fatalf("commands = %#v, want %#v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("commands = %#v, want %#v", got, want)
		}
	}
}
