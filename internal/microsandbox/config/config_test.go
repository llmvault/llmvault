package config

import (
	"reflect"
	"testing"
)

func TestPreviewPortsEnvOverride(t *testing.T) {
	t.Setenv("HIVY_MICROSANDBOX_DEFAULT_PREVIEW_PORTS", "3000, 5173,3000, bad, 70000, 8080")
	cfg := Load()
	want := []int{3000, 5173, 8080}
	if !reflect.DeepEqual(cfg.DefaultPreviewPorts, want) {
		t.Fatalf("DefaultPreviewPorts = %v, want %v", cfg.DefaultPreviewPorts, want)
	}
}
