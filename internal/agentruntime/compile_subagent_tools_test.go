package agentruntime

import (
	"reflect"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

// An empty sub-agent tool selection must default to the read-only set instead of
// compiling to zero (useless) tools; the specs follow runtimeToolOrder order.
func TestBuildSubAgentRuntimeTools_EmptyDefaultsToReadOnly(t *testing.T) {
	tools, err := buildSubAgentRuntimeTools(model.JSON{})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	want := []string{
		"builtin.read_file",
		"builtin.file_search",
		"builtin.glob",
		"builtin.grep",
	}
	if got := runtimeToolTypes(tools); !reflect.DeepEqual(got, want) {
		t.Fatalf("tools = %#v, want %#v", got, want)
	}
}

// A non-empty selection is compiled unchanged (no read-only defaulting).
func TestBuildSubAgentRuntimeTools_NonEmptyUnchanged(t *testing.T) {
	tools, err := buildSubAgentRuntimeTools(model.JSON{"bash": true})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if got := runtimeToolTypes(tools); !reflect.DeepEqual(got, []string{"builtin.bash"}) {
		t.Fatalf("tools = %#v, want [builtin.bash]", got)
	}
}
