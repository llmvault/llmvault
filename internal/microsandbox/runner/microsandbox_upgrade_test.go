package runner

import "testing"

func TestUpgradePortBindingsPreservesExistingHostPorts(t *testing.T) {
	backend := &MicrosandboxBackend{
		ports: map[string]map[int]int{
			"sbx_upgrade": {7080: 47080, 3000: 43000},
		},
	}

	bindings, err := backend.upgradePortBindings("sbx_upgrade", UpgradeSandboxRequest{
		PreviewPorts: []int{3000, 7080},
	})
	if err != nil {
		t.Fatalf("upgradePortBindings: %v", err)
	}
	byGuest := map[int]int{}
	for _, binding := range bindings {
		byGuest[binding.GuestPort] = binding.HostPort
	}
	if byGuest[7080] != 47080 || byGuest[3000] != 43000 {
		t.Fatalf("bindings = %+v", bindings)
	}
}

func TestUpgradePortBindingsRejectsPreviewPortChanges(t *testing.T) {
	backend := &MicrosandboxBackend{
		ports: map[string]map[int]int{
			"sbx_upgrade": {7080: 47080},
		},
	}

	if _, err := backend.upgradePortBindings("sbx_upgrade", UpgradeSandboxRequest{
		PreviewPorts: []int{7080, 3000},
	}); err == nil {
		t.Fatal("upgradePortBindings succeeded with changed preview ports")
	}
}
