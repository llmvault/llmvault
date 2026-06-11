package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
)

// flakyProvider implements only ID/Validate; embedding the interface gives nil
// implementations for the rest (unused by validateSandboxProvider).
type flakyProvider struct {
	sandbox.Provider
	failUntil int // number of initial Validate calls that fail
	calls     int
}

func (p *flakyProvider) ID() string { return "flaky" }

func (p *flakyProvider) Validate(context.Context) error {
	p.calls++
	if p.calls <= p.failUntil {
		return errors.New("transient provider boot failure")
	}
	return nil
}

// A flaky provider that fails early probes but recovers must be retried until it
// succeeds rather than permanently disabling orchestration.
func TestValidateSandboxProviderRetriesTransientFailure(t *testing.T) {
	p := &flakyProvider{failUntil: 2}
	if err := validateSandboxProvider(context.Background(), &config.Config{}, p); err != nil {
		t.Fatalf("validateSandboxProvider = %v, want nil after recovery", err)
	}
	if p.calls != 3 {
		t.Fatalf("Validate called %d times, want 3 (2 failures + 1 success)", p.calls)
	}
}

// A persistently failing provider must surface an error (failing bootstrap).
func TestValidateSandboxProviderFailsAfterExhaustingRetries(t *testing.T) {
	p := &flakyProvider{failUntil: 1000}
	cfg := &config.Config{Environment: "production"}
	if err := validateSandboxProvider(context.Background(), cfg, p); err == nil {
		t.Fatal("expected error when provider never validates")
	}
	if p.calls < 2 {
		t.Fatalf("expected multiple retry attempts, got %d", p.calls)
	}
}

// The backoff loop must abort on context cancellation.
func TestValidateSandboxProviderHonorsContextCancellation(t *testing.T) {
	p := &flakyProvider{failUntil: 1000}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateSandboxProvider(ctx, &config.Config{}, p); err == nil {
		t.Fatal("expected error on cancelled context")
	}
}
