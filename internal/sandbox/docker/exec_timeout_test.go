package docker

import (
	"strings"
	"testing"
	"time"
)

// A timed exec must wrap the command in `timeout` so the in-container process is
// killed at the deadline, not just our hijacked read torn down by ctx cancel.
func TestWrapWithTimeout(t *testing.T) {
	got := wrapWithTimeout("echo hi", 30*time.Second)
	want := "timeout -k 5s 30s /bin/sh -c 'echo hi'"
	if got != want {
		t.Fatalf("wrapWithTimeout = %q, want %q", got, want)
	}
}

// A sub-second timeout still arms
// `timeout` with at least 1s rather than `timeout 0s` (which would never fire).
func TestWrapWithTimeoutRoundsUpSubSecond(t *testing.T) {
	got := wrapWithTimeout("sleep 5", 200*time.Millisecond)
	if !strings.HasPrefix(got, "timeout -k 5s 1s ") {
		t.Fatalf("wrapWithTimeout = %q, want timeout armed with 1s", got)
	}
}

// Commands containing single quotes must be safely quoted so the `timeout`/sh
// wrapper does not break or allow injection out of the quoted argument.
func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	got := shellQuote(`echo 'don't'`)
	want := `'echo '\''don'\''t'\'''`
	if got != want {
		t.Fatalf("shellQuote = %q, want %q", got, want)
	}
}
