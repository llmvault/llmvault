package handler_test

import (
	"context"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/google/uuid"

	"github.com/usehivy/hivy/internal/model"
)

type stubOrgAgentSyncer struct {
	calls int
	orgID uuid.UUID
	err   error
}

func (s *stubOrgAgentSyncer) SyncOrgHivyAgent(_ context.Context, orgID uuid.UUID) error {
	s.calls++
	s.orgID = orgID
	return s.err
}

func TestOrgUpdate_SyncTrueRunsAgentSync(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")
	syncer := &stubOrgAgentSyncer{}
	h.orgHandler.SetAgentSyncer(syncer)

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"website": "https://acme.example",
		"sync":    true,
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}
	if syncer.calls != 1 {
		t.Fatalf("sync calls = %d, want 1", syncer.calls)
	}
	if syncer.orgID != org.ID {
		t.Fatalf("sync org id = %s, want %s", syncer.orgID, org.ID)
	}

	var reloaded model.Org
	if err := h.db.First(&reloaded, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Website != "https://acme.example" {
		t.Errorf("db website: got %q", reloaded.Website)
	}
}

func TestOrgUpdate_SyncFailureSavesFields(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")
	syncer := &stubOrgAgentSyncer{err: errors.New("runtime unavailable")}
	h.orgHandler.SetAgentSyncer(syncer)

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"prompt_company": " Runs field service operations. ",
		"sync":           true,
	})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s, want 400", rr.Code, rr.Body.String())
	}
	if !strings.Contains(rr.Body.String(), "failed to start Hivy agent sandbox") {
		t.Fatalf("body = %s, want sandbox retry error", rr.Body.String())
	}
	if syncer.calls != 1 {
		t.Fatalf("sync calls = %d, want 1", syncer.calls)
	}

	var reloaded model.Org
	if err := h.db.First(&reloaded, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.PromptCompany != "Runs field service operations." {
		t.Errorf("db prompt_company: got %q", reloaded.PromptCompany)
	}
}

func TestOrgUpdate_SyncTrueWithoutConfiguredSyncerSavesFields(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"website": "https://retry.example",
		"sync":    true,
	})

	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status: got %d body=%s, want 400", rr.Code, rr.Body.String())
	}

	var reloaded model.Org
	if err := h.db.First(&reloaded, "id = ?", org.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if reloaded.Website != "https://retry.example" {
		t.Errorf("db website: got %q", reloaded.Website)
	}
}
