package runner

import "testing"

func TestIsMicrosandboxCommandForSandboxName(t *testing.T) {
	raw := []byte("/root/.microsandbox/bin/msb\x00sandbox\x00--name\x00gmfg4zfw\x00")
	if !isMicrosandboxCommandFor(raw, "gmfg4zfw") {
		t.Fatal("expected msb sandbox command to match sandbox id")
	}
}

func TestIsMicrosandboxCommandForSandboxNameEqualsForm(t *testing.T) {
	raw := []byte("/root/.microsandbox/bin/msb\x00sandbox\x00--name=gmfg4zfw\x00")
	if !isMicrosandboxCommandFor(raw, "gmfg4zfw") {
		t.Fatal("expected --name=<id> form to match sandbox id")
	}
}

func TestIsMicrosandboxCommandRejectsOtherSandbox(t *testing.T) {
	raw := []byte("/root/.microsandbox/bin/msb\x00sandbox\x00--name\x00other\x00")
	if isMicrosandboxCommandFor(raw, "gmfg4zfw") {
		t.Fatal("unexpected match for different sandbox id")
	}
}

func TestActualSandboxStateHealthPredicates(t *testing.T) {
	healthy := actualSandboxState{ProcessPIDs: []int{123}, RuntimeHealthChecked: true, RuntimeHealthy: true}
	if !healthy.healthyRunning() {
		t.Fatal("healthy state should be running")
	}
	if healthy.fullyStopped() {
		t.Fatal("healthy state should not be stopped")
	}

	stale := actualSandboxState{ProcessPIDs: []int{123}, RuntimeHealthChecked: true, RuntimeHealthy: false}
	if stale.healthyRunning() {
		t.Fatal("stale state should not be healthy running")
	}
	if !stale.hasHostResidue() {
		t.Fatal("stale state should report host residue")
	}

	stopped := actualSandboxState{}
	if !stopped.fullyStopped() {
		t.Fatal("empty state should be fully stopped")
	}
}
