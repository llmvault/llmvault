package agentruntime

import (
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestBuildRuntimeToolsIncludesLockedDriveTools(t *testing.T) {
	tools, err := buildRuntimeToolsFromSelection(model.JSON{
		"drive_upload":   true,
		"drive_download": true,
	})
	if err != nil {
		t.Fatalf("build runtime tools: %v", err)
	}
	if len(tools) != 2 {
		t.Fatalf("tool count = %d, want 2: %#v", len(tools), tools)
	}
	if tools[0]["type"] != "builtin.drive_upload" || tools[1]["type"] != "builtin.drive_download" {
		t.Fatalf("drive tool order/types = %#v", tools)
	}
	for _, spec := range tools {
		config, ok := spec["config"].(map[string]any)
		if !ok || config["max_file_size_bytes"] != 100*1024*1024 {
			t.Fatalf("drive tool config = %#v", spec["config"])
		}
	}
}

func TestMergeLockedDriveRuntimeToolsCannotBeDisabled(t *testing.T) {
	merged := mergeLockedDriveRuntimeTools(model.JSON{
		"drive_upload":   false,
		"drive_download": map[string]any{"enabled": false},
		"bash":           true,
	})
	if merged["drive_upload"] != true || merged["drive_download"] != true || merged["bash"] != true {
		t.Fatalf("merged selection = %#v", merged)
	}
}
