package handler_test

import (
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
)

// RAG sources are never auto-created. Setting an org's website (and then
// updating the org) must NOT bootstrap a website RAG source or enqueue an
// ingest — sources exist only when created explicitly via POST /v1/rag/sources.
func TestOrgUpdate_DoesNotAutoCreateWebsiteRAGSource(t *testing.T) {
	h := newOrgUpdateHarness(t)
	org, user := h.createOrg(t, "admin")
	if err := h.db.Model(&model.Org{}).Where("id = ?", org.ID).
		Update("website", "https://acme.example").Error; err != nil {
		t.Fatalf("seed existing website: %v", err)
	}

	rr := h.doPatch(t, user.ID, org.ID, "admin", map[string]any{
		"prompt_company": " Runs field service operations. ",
	})

	if rr.Code != http.StatusOK {
		t.Fatalf("status: got %d body=%s, want 200", rr.Code, rr.Body.String())
	}

	var sources []ragmodel.RAGSource
	if err := h.db.Where("org_id = ? AND kind = ?", org.ID, ragmodel.RAGSourceKindWebsite).Find(&sources).Error; err != nil {
		t.Fatalf("load rag sources: %v", err)
	}
	if len(sources) != 0 {
		t.Fatalf("website rag sources = %d, want 0 (no auto-creation)", len(sources))
	}

	if tasks := h.enqueuer.Tasks(); len(tasks) != 0 {
		t.Fatalf("enqueued tasks = %d, want 0 (no auto-ingest)", len(tasks))
	}
}
