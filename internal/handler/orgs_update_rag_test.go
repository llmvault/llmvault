package handler_test

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/usehivy/hivy/internal/model"
	ragmodel "github.com/usehivy/hivy/internal/rag/model"
	ragtasks "github.com/usehivy/hivy/internal/rag/tasks"
)

func TestOrgUpdate_EnsuresWebsiteRAGSourceForExistingWebsite(t *testing.T) {
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
	if len(sources) != 1 {
		t.Fatalf("website rag sources = %d, want 1", len(sources))
	}
	if sources[0].ConfigValue["url"] != "https://acme.example" {
		t.Fatalf("website rag source url = %v, want https://acme.example", sources[0].ConfigValue["url"])
	}

	tasks := h.enqueuer.Tasks()
	if len(tasks) != 1 {
		t.Fatalf("enqueued tasks = %d, want 1", len(tasks))
	}
	if tasks[0].TypeName != ragtasks.TypeRagIngest {
		t.Fatalf("task type = %q, want %q", tasks[0].TypeName, ragtasks.TypeRagIngest)
	}
	var payload ragtasks.IngestPayload
	if err := json.Unmarshal(tasks[0].Payload, &payload); err != nil {
		t.Fatalf("unmarshal ingest payload: %v", err)
	}
	if payload.RAGSourceID != sources[0].ID {
		t.Fatalf("rag_source_id = %s, want %s", payload.RAGSourceID, sources[0].ID)
	}
}
