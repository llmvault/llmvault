package bootstrap

import (
	"context"
	"errors"
	"testing"

	"github.com/usehivy/hivy/internal/config"
	"github.com/usehivy/hivy/internal/sandbox"
)

// flakyProvider satisfies sandbox.Provider but only implements ID/Validate;
// every other method is unused by validateSandboxProvider. Embedding the
// interface gives nil implementations that would panic if called.
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

// TestValidateSandboxProviderRetriesTransientFailure verifies P1-20: a flaky
// provider that fails the first couple of probes but recovers is retried until
// it succeeds rather than permanently disabling orchestration.
func TestValidateSandboxProviderRetriesTransientFailure(t *testing.T) {
	p := &flakyProvider{failUntil: 2}
	if err := validateSandboxProvider(context.Background(), &config.Config{}, p); err != nil {
		t.Fatalf("validateSandboxProvider = %v, want nil after recovery", err)
	}
	if p.calls != 3 {
		t.Fatalf("Validate called %d times, want 3 (2 failures + 1 success)", p.calls)
	}
}

// TestValidateSandboxProviderFailsAfterExhaustingRetries verifies a persistently
// failing provider surfaces an error (which fails bootstrap in production).
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

// TestValidateSandboxProviderHonorsContextCancellation verifies the backoff
// loop aborts when the context is cancelled.
func TestValidateSandboxProviderHonorsContextCancellation(t *testing.T) {
	p := &flakyProvider{failUntil: 1000}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := validateSandboxProvider(ctx, &config.Config{}, p); err == nil {
		t.Fatal("expected error on cancelled context")
	}
}
