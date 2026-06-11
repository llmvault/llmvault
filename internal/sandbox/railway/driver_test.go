package railway

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/usehivy/hivy/internal/sandbox"
)

// TestStopAndArchiveReturnUnsupported verifies P1-13: Railway has no pause/cold
// primitive, so Stop/Archive must return a typed ErrUnsupported instead of a
// silent nil that would let callers persist a 'stopped'/'archived' lie while the
// service keeps billing.
func TestStopAndArchiveReturnUnsupported(t *testing.T) {
	d := &Driver{}
	if err := d.StopSandbox(context.Background(), "svc-1"); !errors.Is(err, sandbox.ErrUnsupported) {
		t.Fatalf("StopSandbox err = %v, want ErrUnsupported", err)
	}
	if err := d.ArchiveSandbox(context.Background(), "svc-1"); !errors.Is(err, sandbox.ErrUnsupported) {
		t.Fatalf("ArchiveSandbox err = %v, want ErrUnsupported", err)
	}
}

func TestRuntimeCommandHTTPTimeoutExceedsCommandTimeout(t *testing.T) {
	commandTimeout := 60 * time.Minute

	got := runtimeCommandHTTPTimeout(commandTimeout)
	want := commandTimeout + runtimeCommandHTTPTimeoutPadding

	if got != want {
		t.Fatalf("runtime command HTTP timeout = %s, want %s", got, want)
	}
}

func TestRuntimeCommandHTTPTimeoutUsesRuntimeDefault(t *testing.T) {
	got := runtimeCommandHTTPTimeout(0)
	want := 120*time.Second + runtimeCommandHTTPTimeoutPadding

	if got != want {
		t.Fatalf("runtime command HTTP timeout = %s, want %s", got, want)
	}
}
