package config

import "testing"

func TestLoadDefaultsRunnerDiskOvercommitToFour(t *testing.T) {
	t.Setenv("HIVY_MICROSANDBOX_RUNNER_DISK_OVERCOMMIT", "")

	cfg := Load()
	if cfg.RunnerDiskOvercommit != 4 {
		t.Fatalf("RunnerDiskOvercommit = %.2f, want 4", cfg.RunnerDiskOvercommit)
	}
}
