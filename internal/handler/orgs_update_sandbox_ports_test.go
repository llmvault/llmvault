package handler_test

import (
	"encoding/json"
	"net/http"
	"slices"
	"testing"

	"github.com/usehivy/hivy/internal/model"
)

func TestOrgUpdate_SandboxExposedPortsSucceed(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"sandbox_exposed_ports": []int{8080, 3000, 3000},
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var got struct {
		SandboxExposedPorts []int `json:"sandbox_exposed_ports"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	want := []int{3000, 8080}
	if !slices.Equal(got.SandboxExposedPorts, want) {
		t.Fatalf("sandbox_exposed_ports = %v, want %v", got.SandboxExposedPorts, want)
	}

	var reloaded model.Org
	if err := h.db.First(&reloaded, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if !slices.Equal(model.SandboxExposedPortsFromInt64Array(reloaded.SandboxExposedPorts), want) {
		t.Fatalf("db sandbox_exposed_ports = %v, want %v", reloaded.SandboxExposedPorts, want)
	}
}

func TestOrgUpdate_SandboxExposedPortsRejectsRuntimePort(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"sandbox_exposed_ports": []int{model.SandboxRuntimeReservedPort},
	})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s, want 400", rr.Code, rr.Body.String())
	}
}
