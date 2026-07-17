package main

import (
	"reflect"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestSandboxRuntimeCreateParamsStartsRuntime(t *testing.T) {
	size := model.TemplateSize{Name: "small", CPU: 1, Memory: 2, Disk: 10}
	params := sandboxRuntimeCreateParams("runtime-small", "runtime:v1", size, 5)

	if params.Name != "runtime-small" {
		t.Fatalf("name = %q, want runtime-small", params.Name)
	}
	if params.Image != "runtime:v1" {
		t.Fatalf("image = %v, want runtime:v1", params.Image)
	}
	wantEntrypoint := []string{
		"/usr/local/bin/hivy-daytona-entrypoint",
	}
	if !reflect.DeepEqual(params.Entrypoint, wantEntrypoint) {
		t.Fatalf("entrypoint = %#v, want %#v", params.Entrypoint, wantEntrypoint)
	}
	if params.Resources == nil {
		t.Fatal("resources must be set")
	}
	if params.Resources.CPU != 1 || params.Resources.Memory != 2 || params.Resources.Disk != 10 {
		t.Fatalf("resources = %+v, want cpu=1 memory=2 disk=10", *params.Resources)
	}
}

func TestSandboxRuntimeCreateParamsCapsDiskAtDaytonaAccountMaximum(t *testing.T) {
	size := model.TemplateSize{Name: "medium", CPU: 2, Memory: 4, Disk: 20}
	params := sandboxRuntimeCreateParams("runtime-medium", "runtime:v1", size, 5)

	if params.Resources == nil {
		t.Fatal("resources must be set")
	}
	if params.Resources.Disk != 10 {
		t.Fatalf("disk = %d, want 10", params.Resources.Disk)
	}
}

func TestSandboxRuntimeCreateParamsCapsXlargeAtDaytonaAccountMaximum(t *testing.T) {
	size := model.TemplateSize{Name: "xlarge", CPU: 8, Memory: 16, Disk: 60}
	params := sandboxRuntimeCreateParams("runtime-xlarge", "runtime:v1", size, 5)

	if params.Resources == nil {
		t.Fatal("resources must be set")
	}
	if params.Resources.CPU != 4 || params.Resources.Memory != 8 || params.Resources.Disk != 10 {
		t.Fatalf("resources = %+v, want cpu=4 memory=8 disk=10", *params.Resources)
	}
}

func TestSandboxRuntimeCreateParamsAdjustsMicroForDaytona(t *testing.T) {
	size := model.TemplateSize{Name: "micro", CPU: 1, Memory: 0, Disk: 5}
	params := sandboxRuntimeCreateParams("runtime-micro", "runtime:v1", size, 5)

	if params.Resources == nil {
		t.Fatal("resources must be set")
	}
	if params.Resources.CPU != 1 || params.Resources.Memory != 1 || params.Resources.Disk != 5 {
		t.Fatalf("resources = %+v, want cpu=1 memory=1 disk=5", *params.Resources)
	}
}

func TestSandboxRuntimeCreateParamsGivesDeveloperNanoEnoughDisk(t *testing.T) {
	size := model.TemplateSize{Name: "nano", CPU: 1, Memory: 1, Disk: 5}
	params := sandboxRuntimeCreateParams("developers-nano", "developers:v1", size, 10)

	if params.Resources == nil {
		t.Fatal("resources must be set")
	}
	if params.Resources.Disk != 10 {
		t.Fatalf("disk = %d, want 10", params.Resources.Disk)
	}
}

func TestResolveSizesIncludesDaytonaMicroAndXlarge(t *testing.T) {
	got, err := resolveSizes("all")
	if err != nil {
		t.Fatalf("resolve all sizes: %v", err)
	}
	want := []string{"micro", "nano", "small", "medium", "large", "xlarge"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("sizes = %#v, want %#v", got, want)
	}
}
