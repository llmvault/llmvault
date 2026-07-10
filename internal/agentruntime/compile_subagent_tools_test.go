package agentruntime

import (
	"reflect"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// An empty sub-agent tool selection must default to read_file instead of
// compiling to zero (useless) tools.
func TestBuildSubAgentRuntimeTools_EmptyDefaultsToReadFile(t *testing.T) {
	tools, err := buildSubAgentRuntimeTools(model.JSON{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := []string{"builtin.read_file"}
	if got := runtimeToolTypes(tools); !reflect.DeepEqual(got, want) {
		t.Fatalf("tools = %#v, want %#v", got, want)
	}
}

// A non-empty selection is compiled unchanged (no read-only defaulting).
func TestBuildSubAgentRuntimeTools_NonEmptyUnchanged(t *testing.T) {
	tools, err := buildSubAgentRuntimeTools(model.JSON{"read_file": true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := runtimeToolTypes(tools); !reflect.DeepEqual(got, []string{"builtin.read_file"}) {
		t.Fatalf("tools = %#v, want [builtin.read_file]", got)
	}
}
