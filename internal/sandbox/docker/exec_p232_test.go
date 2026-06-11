package docker

import (
	"strings"
	"testing"
	"time"
)

// TestWrapWithTimeout verifies P2-32: a timed exec wraps the command in
// `timeout` so the in-container process is killed when the deadline fires,
// rather than relying solely on ctx cancellation (which only tears down our
// hijacked read).
func TestWrapWithTimeout(t *testing.T) {
	got := wrapWithTimeout("echo hi", 30*time.Second)
	want := "timeout -k 5s 30s /bin/sh -c 'echo hi'"
	if got != want {
		t.Fatalf("wrapWithTimeout = %q, want %q", got, want)
	}
}

// TestWrapWithTimeoutRoundsUpSubSecond verifies a sub-second timeout still arms
// `timeout` with at least 1s rather than `timeout 0s` (which would never fire).
func TestWrapWithTimeoutRoundsUpSubSecond(t *testing.T) {
	got := wrapWithTimeout("sleep 5", 200*time.Millisecond)
	if !strings.HasPrefix(got, "timeout -k 5s 1s ") {
		t.Fatalf("wrapWithTimeout = %q, want timeout armed with 1s", got)
	}
}

// TestShellQuoteEscapesSingleQuotes verifies commands containing single quotes
// are safely quoted so the `timeout`/sh wrapper does not break or allow
// injection out of the quoted argument.
func TestShellQuoteEscapesSingleQuotes(t *testing.T) {
	got := shellQuote(`echo 'don't'`)
	want := `'echo '\''don'\''t'\'''`
	if got != want {
		t.Fatalf("shellQuote = %q, want %q", got, want)
	}
}
